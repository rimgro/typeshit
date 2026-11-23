#!/usr/bin/env python3
"""Fetch the texttoimages.pro handwriting font catalogue for parity debugging.

Writes TTF files plus catalog.json into a directory (default: data/fonts), which
the server picks up via `-fonts <dir>`. Purely a manual debug helper: the files
are third-party fonts, so keep them out of the repo (data/ is gitignored).

    python3 scripts/fetch-parity-fonts.py [--dir data/fonts] [--only ru]

The upstream catalogue endpoint is a public JSON list:
    GET https://texttoimages.pro/api/fonts
    GET https://texttoimages.pro/api/fonts/{id}/file   -> font/ttf
"""
from __future__ import annotations

import argparse
import json
import os
import sys
import time
import urllib.request

BASE = "https://texttoimages.pro"
HEADERS = {"User-Agent": "Mozilla/5.0", "Accept-Encoding": "identity"}
MAGIC = (b"\x00\x01\x00\x00", b"OTTO", b"true", b"ttcf")


def fetch(url: str, binary: bool, tries: int = 4):
    last = None
    for attempt in range(tries):
        try:
            req = urllib.request.Request(url, headers=HEADERS)
            with urllib.request.urlopen(req, timeout=180) as resp:
                body = resp.read()
            return body if binary else json.loads(body)
        except Exception as exc:  # network flake, truncated chunked body, ...
            last = exc
            time.sleep(1.5 * (attempt + 1))
    raise RuntimeError(f"{url}: {last}")


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--dir", default="data/fonts", help="output directory")
    ap.add_argument("--only", default=None, help="keep fonts whose languages contain this tag (e.g. ru)")
    ap.add_argument("--force", action="store_true", help="re-download existing files")
    args = ap.parse_args()

    os.makedirs(args.dir, exist_ok=True)
    catalogue = fetch(f"{BASE}/api/fonts", binary=False)
    if args.only:
        catalogue = [f for f in catalogue if args.only in (f.get("languages") or [])]

    entries, failed = [], []
    for font in catalogue:
        fid = font["id"]
        path = os.path.join(args.dir, fid + ".ttf")
        if args.force or not os.path.exists(path) or os.path.getsize(path) < 1024:
            try:
                data = fetch(f"{BASE}/api/fonts/{fid}/file", binary=True)
            except Exception as exc:
                print(f"  !! {fid}: {exc}", file=sys.stderr)
                failed.append(fid)
                continue
            if data[:4] not in MAGIC:
                print(f"  !! {fid}: not a TrueType/OpenType file", file=sys.stderr)
                failed.append(fid)
                continue
            with open(path, "wb") as fh:
                fh.write(data)
        entries.append({
            "id": fid,
            "name": font.get("name") or fid,
            "category": font.get("category") or "handwriting",
            "source": "texttoimages.pro",
            "scripts": font.get("scripts") or [],
            "languages": font.get("languages") or [],
            "origin": font.get("description") or "",
            "file": font.get("file") or (fid + ".ttf"),
        })
        print(f"  {fid:<16} {entries[-1]['name']:<12} [{entries[-1]['category']}]")

    meta = {
        "generatedBy": "scripts/fetch-parity-fonts.py",
        "note": "Local parity-debug font directory. Third-party fonts: do not commit.",
        "fonts": entries,
    }
    with open(os.path.join(args.dir, "catalog.json"), "w") as fh:
        json.dump(meta, fh, ensure_ascii=False, indent=1)

    print(f"\n{len(entries)} font(s) in {args.dir}" + (f", {len(failed)} failed" if failed else ""))
    return 1 if failed else 0


if __name__ == "__main__":
    raise SystemExit(main())
