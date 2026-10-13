# Parsing

## Purpose
Turns one file on disk into normalized blocks with locators: the block model, the parser registry and the local parsers. It does not choose between providers, call Datalab, fetch objects or write to the database; `ParseService` (Task 3a-5) and `ingest/` do that.

## Entry points
- `model.py`: `Block`, `BlockType`, `Locator`, `LocatorKind`, `TableData`, `ParsedDocument`, `ParseProvider`, and `BlockCollector` (numbers blocks `b-0001`, `b-0002`, ... in document order and drops blank text).
- `registry.py`: `Parser` protocol (`parse(path) -> ParsedDocument`, synchronous; callers run it in a thread), `ParserRegistry.for_extension`, `local_registry()`, `IMAGE_EXTENSIONS`.
- `text_files.py` (`.txt`, `.md`), `sheets.py` (`.csv`, `.xlsx`), `html_files.py` (`.html`, `.htm`), `office.py` (`.docx`, `.pptx`), `pdf_local.py` (`.pdf`).

## Dependencies
- Uses: python-docx, python-pptx, openpyxl, PyMuPDF, beautifulsoup4, markdown-it-py.
- Used by: `ParseService` and the ingest pipeline (Tasks 3a-5, 3a-6); the Datalab adapter reuses the model.

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

**Fallback and `degraded`.** Parsers never set `degraded`; `ParseService` does, and only when a local parser replaced Datalab for a type that Datalab handles first.

**Adding a format.** Write a class with `parse(path) -> ParsedDocument`, give every block one of the four locator kinds, and add the extension to `local_registry()`. Add the extension to `SUPPORTED_EXTENSIONS` (`kb/upload.py`) and `contracts/kb-upload.json` separately.

**HTML** is read through `html.parser` (it keeps source lines). Main blocks are `h1`-`h6`, `p`, `pre`, `blockquote`, `ul`/`ol` and `table`. Text that sits directly in a container such as `div`, `section` or `article` (a page built without `p`) becomes a `text` block per run between block tags, so such a page is not empty; text inside a block tag is never read twice. `<br>` and form controls (`label`, `input`, `select`, `option`, `button`, ...) separate words (`<br>` is a newline inside `pre`). The walk is iterative, so a malformed page nested thousands of levels deep (html.parser nests unclosed tags) still parses. HTML tables are not span-expanded.

Parsers let library exceptions escape; `ParseService` maps them to `INGEST_PARSE_FAILED`. Text files are decoded as UTF-32 or UTF-16 when they start with a byte order mark, else UTF-8, then cp1258, then latin-1, and normalized to NFC. `BlockCollector` strips NUL characters (Postgres text and jsonb reject them; PDF text layers can carry them).

Datalab heading levels (Task 3a-5) are layout-guessed: in the S9 spike DOCX levels matched Word styles in 6 of 12 cases, and a 5-page PDF came back as `h6` on every page. Do not rely on them where a local parser can read the real style.

## Common failures
- A DOCX or PPTX gives `INGEST_NO_TEXT` → its text is inside pictures → needs Datalab (external parsing on and a key set).
- A DOCX citation points at the wrong paragraph → the index counts tables and empty paragraphs too (python-docx body order).
- A formula cell in an `.xlsx` is empty → the file has no cached result (never calculated by Excel); openpyxl `data_only` reads none.
