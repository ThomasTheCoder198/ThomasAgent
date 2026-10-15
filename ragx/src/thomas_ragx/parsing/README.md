# Parsing

## Purpose
Turns one file on disk into normalized blocks with locators: the block model, the parser registry, the local parsers, the Datalab adapter and the parse policy. It does not fetch objects or write to the database; `ingest/` does that.

## Entry points
- `model.py`: `Block`, `BlockType`, `Locator`, `LocatorKind`, `TableData`, `ParsedDocument`, `ParseProvider`, and `BlockCollector` (numbers blocks `b-0001`, `b-0002`, ... in document order and drops blank text).
- `registry.py`: `Parser` protocol (`parse(path) -> ParsedDocument`, synchronous; callers run it in a thread), `ParserRegistry.for_extension`, `local_registry()`, `IMAGE_EXTENSIONS`.
- `service.py`: `ParseService.parse(path, extension, *, external_parsing) -> ParsedDocument`, the policy below. `datalab.py`: `DatalabConverter` (the SDK call, `ConvertOutcome`) and `create_converter(config)` (`None` without a usable key). `datalab_blocks.py`: Datalab JSON tree to blocks (`to_parsed_document`, `table_from_html`, `text_from_html`). `page_sizes.py`: source page sizes that turn Datalab pixel boxes into points. `locators.py`: `DATALAB_LOCATOR_KINDS`.
- `text_files.py` (`.txt`, `.md`), `sheets.py` (`.csv`, `.xlsx`), `html_files.py` (`.html`, `.htm`), `office.py` (`.docx`, `.pptx`), `pdf_local.py` (`.pdf`).

## Dependencies
- Uses: python-docx, python-pptx, openpyxl, PyMuPDF, beautifulsoup4, markdown-it-py.
- Uses also: `datalab-python-sdk` (key and address always passed explicitly; ragx never reads `DATALAB_*`), OpenTelemetry (spans `rag.parse.datalab`, `rag.parse.local`).
- Used by: the ingest pipeline (Task 3a-6) through `ParseService`.

## Run & test
```bash
cd ragx && uv run pytest -q tests/unit/parsing
```

## Conventions
**Locators.** One kind per file type; `Locator.to_json()` omits empty fields and uses the keys `kind`, `page`, `bbox`, `sheet`, `range`, `lineStart`, `lineEnd`, `paragraph`.

| Kind | Used for | Basis |
|---|---|---|
| `pdf_bbox` | PDF, PPTX (page = slide), images from Datalab; also a DOCX or PPTX that Datalab rescues (S9-1, O6-2) | pages and slides are 1-based; `bbox` is `(x0, y0, x1, y1)` in points with a top-left origin. On a rotated PDF page the local parser reports the box in the unrotated page space (not transformed) |
| `docx_paragraph` | DOCX | the 0-based index in python-docx's `iter_inner_content()` (paragraphs and tables in body order) |
| `text_lines` | `.txt`, `.md`, `.html` | 1-based source lines, end inclusive |
| `sheet_range` | `.csv`, `.xlsx` | sheet name and an A1 range; rows are 1-based and keep their source numbers when empty rows are skipped; the range stops at the last non-empty column. A CSV has no sheets: its sheet name is the constant `CSV_SHEET_NAME` (`"csv"`), not the file stem, because the pipeline parses a temp file named `<file_id>-<version>.csv` and the stem would leak an id into every citation |

**Tables** are one `table` block with `TableData(header, rows)`; `block.text` is its markdown table (`table_text`). Merged cells always become a rectangular grid: a cell spanning columns or rows repeats its text in every grid slot it covers (DOCX and PPTX). A DOCX row that starts late or ends early (`w:gridBefore` / `w:gridAfter`) is padded with empty cells, so every row has the header's width. `source_rows` and `columns` (sheets only) let the chunker cite a row group.

**Local DOCX and PPTX are the primary path** for those two types (owner decision O6). They read exact text and exact styles, so the quality bar is that of a first choice, not a fallback.
- DOCX headings come from Word styles: `Title` is level 1 and `Heading N` is level N, also through custom styles based on them. A direct or style-level outline level wins, body text (level 9) included: Word's `TOC Heading` is based on Heading 1 but is not a heading.
- DOCX lists: consecutive list paragraphs (list styles or numbering) become one `list` block located at the first one; depth and decimal-or-bullet kind come from `w:ilvl` and `numbering.xml`.
- PPTX: the title placeholder is the level-1 heading and is emitted first; the other shapes follow top-to-bottom, left-to-right. A text frame is a `list` block when its paragraphs have bullets or auto-numbers (body placeholders show bullets by inheritance); levels indent two spaces. Grouped shapes are read and take the outermost group's box. Charts and SmartArt are not read. Slide notes are a `text` block with a page-only locator. Footer, date and slide-number placeholders are skipped.
- **Embedded pictures yield no text.** There is no OCR on the server (O5): a DOCX or PPTX whose text lives only in pictures parses to no blocks, which ingest turns into `INGEST_NO_TEXT` unless Datalab is enabled to rescue it. Text inside text boxes, SDT content controls, headers, footers, footnotes and nested tables is not read from DOCX.

**Local PDF** is a fallback: text layer only, headings guessed from font size, no tables, no OCR. PyMuPDF is AGPL-3.0 (or an Artifex commercial licence); this is an owner note, not a blocker.

**Parse policy (`ParseService`).**

| File | First | Fallback | `degraded` |
|---|---|---|---|
| PDF, `.png`, `.jpg`, `.jpeg` | Datalab, when the org allows external parsing and a key is set | local parser (PDF: text layer only; an image has none, so `INGEST_NO_TEXT`, or the retryable `PROVIDER_UNAVAILABLE` after a Datalab outage) | `True` when the fallback ran |
| extension in `RAG_LOCAL_FIRST_EXTENSIONS` (default `.docx`, `.pptx`; owner decision O6) | local parser | when it raises `INGEST_PARSE_FAILED` or yields no blocks, and Datalab is enabled, Datalab once | always `False`; the provider is the one that produced the blocks |
| every other extension (`.txt`, `.md`, `.csv`, `.xlsx`, `.html`) | local parser | none | `False` |

- A local parser that raises is mapped to `INGEST_PARSE_FAILED`; no blocks at all is `INGEST_NO_TEXT`. If a Datalab rescue fails too, the local outcome stands.
- **Outage is retryable (ruling C3).** When Datalab was enabled, its convert call raised, and the local parse then has no text (a scan, an image, an empty or picture-only DOCX), `parse` raises the retryable `PROVIDER_UNAVAILABLE` instead of `INGEST_NO_TEXT`, so a transient outage retries rather than dead-lettering on the first delivery. Datalab off (no key, or the org opted out), or Datalab answering with no text, keeps `INGEST_NO_TEXT`. A local `INGEST_PARSE_FAILED` stays as it is. (This departs from plan A6, which went straight to local.)
- **Only the convert call counts as an outage.** If ragx cannot map a result Datalab delivered (a bug, an unexpected shape), the file fails with `INGEST_PARSE_FAILED` and the local parser does not run.
- **Span attributes (ruling C2).** `parse(..., span_attributes=...)` copies the given `request_id`, `org_id`, `kb_id` and `file_id` onto every `rag.parse.*` span. Spans also carry `parse.provider`, `parse.mode`, `parse.page_count`, `parse.quality` (when Datalab gave one), `parse.runtime_seconds` and `parse.cost_cents`. Exceptions are not recorded on spans, and a failed span (`set_status_on_exception=False`, status set by hand) has status `ERROR` and never the exception message, because a provider message can echo content. Where ragx sets the status, the description is the exception type name; in the worker, API and eval processes the LiteLLM exception policy (`platform/tracing.py`) strips status descriptions, so there the span carries `ERROR` with no description. A Datalab success logs `datalab convert completed` with `mode` and `duration_ms`; a failure logs `error_type` and the HTTP `status_code` only, never the message or the key.
- Datalab is enabled when the org has `externalParsing` on **and** `RAG_DATALAB_API_KEY` is non-blank. A blank key counts as no key.
- Quality retry: one more convert in `RAG_DATALAB_RETRY_MODE` only when the score is not `None` and below `RAG_DATALAB_MIN_QUALITY`; the better score wins. A `None` score (DOCX, PPTX, some PDFs) means unknown: it is not retried and is stored as no quality, never 0.0. The score does not catch OCR damage (S9), so treat the threshold as a safety net.
- No extra retries around the SDK. It retries on its own: the submit call only on 429; polling on timeouts, 408, 429, 5xx and errors with no status; both with 10 attempts and 5 to 120 s jittered backoff, ignoring `Retry-After`. `max_polls` is the timeout divided by the poll interval, at least 1 (`Config` requires the timeout to be at least one interval). A Datalab error or timeout (`RAG_DATALAB_TIMEOUT_SECONDS`) goes to the fallback; the failure is logged by type only.
- Datalab retention follows the SDK and the account defaults (owner answer A8). `RAG_DATALAB_SKIP_CACHE` defaults to `false`, the SDK default, so Datalab's result cache stays on; set `true` to send `skip_cache=true`. Image captions stay on (A21): a figure's text is Datalab's synthetic caption.

**Datalab JSON** (shape measured by spike S9, `docs/superpowers/spikes/S9-datalab.md`, git-ignored). The root has no `block_type`; its `children` are `Page` nodes. Leaves carry `id`, `block_type`, `html`, `page`, `bbox`, `polygon` and no `children`. A table is one `Table` with `html`: only the outer table's rows and cells are read (a nested table folds into its cell's text), and `rowspan`/`colspan` are expanded into a rectangular grid, as for local tables, capped at `MAX_TABLE_COLUMNS` (1000) columns and `MAX_TABLE_CELLS` (100000) cells so a hostile span cannot exhaust memory. When the cell cap drops rows or cells the table logs one `table truncated by the size caps` warning with counts only (`rows_in_source`, `rows_kept`, `columns`, `max_cells`, `max_columns`), never cell text. A list is one `ListGroup` block with one line per item in the local shape (`- ` bullets, `1. ` numbers, two spaces per nesting level); `Code` keeps its line breaks and indentation; inline math arrives as `<math>LaTeX</math>` and is unwrapped (`\leq` becomes `≤`, unknown LaTeX stays raw). Page furniture (`PageHeader`, `PageFooter`) is skipped. `TableGroup` and `FigureGroup` children, `Caption`, `Figure` and `Line` are handled but were not seen in the S9 corpus; the real-key run (3c-5) checks them. Pages are numbered by their order in the root, 1-based. There is no line level: the finest citation is a block.

**Datalab boxes are pixels of Datalab's own page rendering** (about 2.7x the point size for PDF, DOCX and PPTX; 1:1 for images), but a `pdf_bbox` locator is points. `ParseService` reads the source page size (`page_sizes.py`: PyMuPDF `page.rect` per PDF page, the python-pptx slide size, the first python-docx section, the pixel size of an image) and scales each axis by `source size / Page node size`. With no readable source size, a block cites its page only (`{"kind": "pdf_bbox", "page": n}`); raw pixels never reach a locator. A DOCX with sections of different sizes, and a JPEG whose EXIF rotates it, are scaled by the first size, so those boxes can be off. A DOCX page is Datalab's pagination, which may differ from Word's.

**Fallback and `degraded`.** Parsers never set `degraded`; `ParseService` does, and only when a local parser replaced Datalab for a type that Datalab handles first (PDF and images). A Datalab rescue of a DOCX or PPTX is not degraded.

**Adding a format.** Write a class with `parse(path) -> ParsedDocument`, give every block one of the four locator kinds, and add the extension to `local_registry()`. Add the extension to `SUPPORTED_EXTENSIONS` (`kb/upload.py`) and `contracts/kb-upload.json` separately.

**HTML** is read through `html.parser` (it keeps source lines). Main blocks are `h1`-`h6`, `p`, `pre`, `blockquote`, `ul`/`ol` and `table`. Text that sits directly in a container such as `div`, `section` or `article` (a page built without `p`) becomes a `text` block per run between block tags, so such a page is not empty; text inside a block tag is never read twice. `<br>` and form controls (`label`, `input`, `select`, `option`, `button`, ...) separate words (`<br>` is a newline inside `pre`). The walk is iterative, so a malformed page nested thousands of levels deep (html.parser nests unclosed tags) still parses. HTML tables are not span-expanded.

Parsers let library exceptions escape; `ParseService` maps them to `INGEST_PARSE_FAILED`. Text files are decoded as UTF-32 or UTF-16 when they start with a byte order mark, else UTF-8, then cp1258, then latin-1, and normalized to NFC. `BlockCollector` strips NUL characters (Postgres text and jsonb reject them; PDF text layers can carry them).

Datalab heading levels are layout-guessed (the `<hN>` tag is kept as is): in the S9 spike DOCX levels matched Word styles in 6 of 12 cases, and a 5-page PDF came back as `h6` on every page. Do not rely on them where a local parser can read the real style.

## Common failures
- A DOCX or PPTX gives `INGEST_NO_TEXT` → its text is inside pictures → needs Datalab (external parsing on and a key set).
- A Datalab citation has a page but no box → the source page size was unreadable (check the file opens locally).
- A DOCX citation points at the wrong paragraph → the index counts tables and empty paragraphs too (python-docx body order).
- A formula cell in an `.xlsx` is empty → the file has no cached result (never calculated by Excel); openpyxl `data_only` reads none.

The local PDF parser opens the verified bytes as a MuPDF stream so corrupt input cannot retain a Windows filename handle and mask the parse error during ingest cleanup. This requires at most one upload-sized byte buffer.
