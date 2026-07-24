#!/usr/bin/env python3
"""Generate sample books for testing e-biblioteca.

Usage:  python3 .agents/skills/testing-e-biblioteca/make_fixtures.py books/test

Writes five files into the target directory (created if missing):

  Long Novel.epub     3 chapters of filler, long enough to paginate in epub.js
  Sample Novel.epub   1 short chapter — deliberately fits a single page
  Sample Report.pdf   1 page, minimal hand-written PDF
  Sample Comic.cbz    pages stored as page10/page2/page1 in distinct solid colours
  Sample Audiobook.mp3  20s tone (requires ffmpeg; skipped with a warning if absent)

The odd bits are deliberate test affordances:
  * CBZ pages are stored out of natural order and colour-coded, so page-ordering
    and cover-selection bugs are visible at a glance (blue=page1, green=page2,
    red=page10).
  * Two EPUBs: the long one exercises pagination/progress, the short one catches
    readers that only save progress on relocation.
"""

import os
import shutil
import struct
import subprocess
import sys
import zipfile
import zlib

CONTAINER_XML = """<?xml version="1.0"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
 <rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles>
</container>"""


def opf(title: str, uid: str, chapters: int) -> str:
    items = "\n".join(
        f'  <item id="c{i}" href="c{i}.xhtml" media-type="application/xhtml+xml"/>'
        for i in range(1, chapters + 1)
    )
    spine = "".join(f'<itemref idref="c{i}"/>' for i in range(1, chapters + 1))
    return f"""<?xml version="1.0" encoding="utf-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="bid">
 <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
  <dc:identifier id="bid">urn:uuid:{uid}</dc:identifier>
  <dc:title>{title}</dc:title>
  <dc:creator>Ada Testwright</dc:creator>
  <dc:language>en</dc:language>
  <dc:publisher>Test Press</dc:publisher>
 </metadata>
 <manifest>
{items}
 </manifest>
 <spine>{spine}</spine>
</package>"""


def chapter(n: int, paragraphs: int, sentence: str) -> str:
    body = "\n".join(f"<p>{sentence * 60}</p>" for _ in range(paragraphs))
    return f"""<?xml version="1.0" encoding="utf-8"?>
<html xmlns="http://www.w3.org/1999/xhtml"><head><title>Chapter {n}</title></head>
<body><h1>Chapter {n}</h1>
{body}
</body></html>"""


def write_epub(path: str, title: str, uid: str, chapters: int, paragraphs: int, sentence: str) -> None:
    with zipfile.ZipFile(path, "w") as z:
        # mimetype must be first and stored uncompressed
        z.writestr("mimetype", "application/epub+zip", compress_type=zipfile.ZIP_STORED)
        z.writestr("META-INF/container.xml", CONTAINER_XML)
        z.writestr("OEBPS/content.opf", opf(title, uid, chapters))
        for i in range(1, chapters + 1):
            z.writestr(f"OEBPS/c{i}.xhtml", chapter(i, paragraphs, sentence))


def solid_png(width: int, height: int, rgb: tuple) -> bytes:
    """Minimal valid PNG of a single solid colour."""

    def chunk(kind: bytes, data: bytes) -> bytes:
        return (
            struct.pack(">I", len(data))
            + kind
            + data
            + struct.pack(">I", zlib.crc32(kind + data) & 0xFFFFFFFF)
        )

    raw = b"".join(b"\x00" + bytes(rgb) * width for _ in range(height))
    return (
        b"\x89PNG\r\n\x1a\n"
        + chunk(b"IHDR", struct.pack(">IIBBBBB", width, height, 8, 2, 0, 0, 0))
        + chunk(b"IDAT", zlib.compress(raw))
        + chunk(b"IEND", b"")
    )


def write_cbz(path: str) -> None:
    # Stored out of natural order on purpose; colours identify the page.
    pages = [
        ("page10.png", (220, 40, 40)),   # red   — must sort LAST
        ("page2.png", (40, 180, 60)),    # green
        ("page1.png", (50, 60, 220)),    # blue  — must sort FIRST and be the cover
    ]
    with zipfile.ZipFile(path, "w") as z:
        for name, rgb in pages:
            z.writestr(name, solid_png(600, 900, rgb))


def write_pdf(path: str) -> None:
    content = b"BT /F1 18 Tf 40 110 Td (Sample PDF page) Tj ET\n"
    objects = [
        b"<< /Type /Catalog /Pages 2 0 R >>",
        b"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
        b"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 200] "
        b"/Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>",
        b"<< /Length %d >>\nstream\n" % len(content) + content + b"endstream",
        b"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
    ]
    out = bytearray(b"%PDF-1.4\n")
    offsets = []
    for i, obj in enumerate(objects, start=1):
        offsets.append(len(out))
        out += b"%d 0 obj\n" % i + obj + b"\nendobj\n"
    xref = len(out)
    out += b"xref\n0 %d\n" % (len(objects) + 1)
    out += b"0000000000 65535 f \n"
    for off in offsets:
        out += b"%010d 00000 n \n" % off
    out += b"trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n" % (
        len(objects) + 1,
        xref,
    )
    with open(path, "wb") as f:
        f.write(bytes(out))


def write_mp3(path: str) -> bool:
    if not shutil.which("ffmpeg"):
        return False
    subprocess.run(
        [
            "ffmpeg", "-y", "-loglevel", "error",
            "-f", "lavfi", "-i", "sine=frequency=440:duration=20",
            "-metadata", "title=Sample Audiobook",
            "-metadata", "artist=Ada Testwright",
            path,
        ],
        check=True,
    )
    return True


def main() -> int:
    target = sys.argv[1] if len(sys.argv) > 1 else "books/test"
    os.makedirs(target, exist_ok=True)

    write_epub(
        os.path.join(target, "Long Novel.epub"),
        "Long Novel", "long-novel-1", chapters=3, paragraphs=12,
        sentence="The quick brown fox jumps over the lazy dog. ",
    )
    write_epub(
        os.path.join(target, "Sample Novel.epub"),
        "Sample Novel", "sample-novel-1", chapters=1, paragraphs=1,
        sentence="Hello reader. ",
    )
    write_pdf(os.path.join(target, "Sample Report.pdf"))
    write_cbz(os.path.join(target, "Sample Comic.cbz"))
    if not write_mp3(os.path.join(target, "Sample Audiobook.mp3")):
        print("warning: ffmpeg not found, skipped Sample Audiobook.mp3", file=sys.stderr)

    for name in sorted(os.listdir(target)):
        print(os.path.join(target, name))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
