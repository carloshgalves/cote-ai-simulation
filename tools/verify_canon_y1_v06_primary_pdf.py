"""Recheck Y1 V6 short evidence anchors against an operator-supplied PDF.

Usage: python tools/verify_canon_y1_v06_primary_pdf.py "COTE Y1 V6.pdf"
Requires poppler pdftotext; does NOT download or commit the copyrighted novel.
This is independent *mechanical page anchoring*, not an independent human
literary opinion and not semantic certification of actors or strategies.
"""
import hashlib
import json
import re
import shutil
import subprocess
import sys
from pathlib import Path

ROOT=Path(__file__).resolve().parents[1]
ANCHOR_PATH=ROOT/"data/canon/consolidated/audits/y1-v06-verbatim-source-anchors.json"

def text_norm(s):
    return re.sub(r"\s+"," ",s).casefold()

def verify(pdf_path,anchor_path=ANCHOR_PATH):
    pdf_path=Path(pdf_path)
    assert pdf_path.is_file(),f"PDF not found: {pdf_path}"
    anchors=json.loads(Path(anchor_path).read_text(encoding="utf-8"))
    digest=hashlib.sha256(pdf_path.read_bytes()).hexdigest()
    assert digest==anchors["pdf_sha256"],f"SHA mismatch: {digest}"
    assert shutil.which("pdftotext"),"pdftotext is required"
    result=subprocess.run(["pdftotext","-layout",str(pdf_path),"-"],
        check=True,capture_output=True,text=True)
    pages=result.stdout.split("\f")
    assert len(pages)-1==anchors["physical_page_count"],f"PDF page count {len(pages)-1}"
    assert len(anchors["records"])==80
    checked=[];failed=[]
    for record in anchors["records"]:
        p=record["pdf_page"];needle=text_norm(record["normalized_source_fragment"])
        if 1<=p<=anchors["physical_page_count"] and needle in text_norm(pages[p-1]):
            checked.append(record["record_id"])
        else:
            failed.append(record["record_id"])
    if failed:raise AssertionError(f"Unmatched anchors: {failed}")
    assert len(set(checked))==80 and len(checked)==80
    return dict(status="PASS",verified_anchors=len(checked),source_sha256=digest,
                operator_pdf_pages=anchors["physical_page_count"],
                caveat="Mechanical substrings and source SHA only; NOT independent semantic signoff")

if __name__=="__main__":
    if len(sys.argv)!=2:raise SystemExit(__doc__)
    print(json.dumps(verify(sys.argv[1]),ensure_ascii=False))
