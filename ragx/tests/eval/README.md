# Synthetic evaluation corpus

This committed fictional corpus is used by CI and the S6 Vietnamese BM25 tokenizer spike. It contains five source files and 34 questions. The owner's real evaluation set lives under git-ignored `docs/` and uses the same format (O4).

Each line of `synthetic.jsonl` is a UTF-8 JSON object with `id`, `lang` (`vi` or `en`), `category`, `question`, and `expected` fields. `expected` is a list of `{ "file": ..., "quote": ... }` objects. Empty `expected` lists are excluded from recall and MRR.

Categories A1–A6 cover greeting and identity (A1), factual retrieval (A2), document summary (A3), out-of-scope questions (A4), unsupported answers (A5), and comparison across evidence (A6). This set includes A1, A2, A4, A5, and A6; it does not claim full category coverage.

Expected quotes are matched against their source after NFC normalization, lowercasing, and whitespace collapsing. The corpus consistency test enforces this relationship.
