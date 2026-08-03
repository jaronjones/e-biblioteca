package metadata

import (
	"archive/zip"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// writeEPUB builds a zip at path with entries in the given order — order
// matters for tests that prove container.xml wins over stray .opf files.
func writeEPUB(t *testing.T, entries [][2]string) string {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, e := range entries {
		f, err := w.Create(e[0])
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write([]byte(e[1])); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "book.epub")
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

const containerXML = `<?xml version="1.0"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`

func TestExtractEPUB3(t *testing.T) {
	opf := `<?xml version="1.0"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:title>Real Book</dc:title>
    <dc:creator>Jane Author</dc:creator>
    <dc:description>A real description.</dc:description>
    <dc:publisher>Test Press</dc:publisher>
    <dc:language>en</dc:language>
    <dc:subject>Testing</dc:subject>
    <dc:identifier>urn:uuid:550e8400-e29b-41d4-a716-446655440000</dc:identifier>
    <dc:identifier id="pub-id">urn:isbn:9780134685991</dc:identifier>
  </metadata>
  <manifest>
    <item id="img1" href="images/decorative.png" media-type="image/png"/>
    <item id="cimg" href="images/cover.jpg" media-type="image/jpeg" properties="cover-image"/>
    <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>
  </manifest>
</package>`
	// decoy .opf placed before container.xml: zip order must not matter
	path := writeEPUB(t, [][2]string{
		{"decoy.opf", `<package><metadata><dc:title xmlns:dc="http://purl.org/dc/elements/1.1/">Wrong Book</dc:title></metadata></package>`},
		{"META-INF/container.xml", containerXML},
		{"OEBPS/content.opf", opf},
		{"OEBPS/images/decorative.png", "PNGDATA"},
		{"OEBPS/images/cover.jpg", "JPGDATA"},
	})

	meta, err := extractEPUB(path)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Title != "Real Book" {
		t.Errorf("title = %q, want Real Book (container.xml OPF must win over decoy.opf)", meta.Title)
	}
	if !reflect.DeepEqual(meta.Authors, []string{"Jane Author"}) {
		t.Errorf("authors = %v", meta.Authors)
	}
	if meta.ISBN13 != "9780134685991" {
		t.Errorf("isbn13 = %q", meta.ISBN13)
	}
	if meta.ISBN10 != "" {
		t.Errorf("isbn10 = %q, want empty (UUID must not be mistaken for ISBN)", meta.ISBN10)
	}
	if string(meta.CoverData) != "JPGDATA" {
		t.Errorf("cover = %q, want JPGDATA (cover-image property, not first image)", meta.CoverData)
	}
	if meta.CoverExt != ".jpg" {
		t.Errorf("cover ext = %q", meta.CoverExt)
	}
}

func TestExtractEPUB2MetaCover(t *testing.T) {
	opf := `<?xml version="1.0"?>
<package xmlns="http://www.idpf.org/2007/opf" version="2.0">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:opf="http://www.idpf.org/2007/opf">
    <dc:title>Older Book</dc:title>
    <dc:identifier opf:scheme="ISBN">0-306-40615-2</dc:identifier>
    <meta name="cover" content="cover-id"/>
  </metadata>
  <manifest>
    <item id="other" href="pic.png" media-type="image/png"/>
    <item id="cover-id" href="cov.jpg" media-type="image/jpeg"/>
  </manifest>
</package>`
	path := writeEPUB(t, [][2]string{
		{"META-INF/container.xml", `<?xml version="1.0"?>
<container xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles><rootfile full-path="content.opf"/></rootfiles>
</container>`},
		{"content.opf", opf},
		{"pic.png", "PICDATA"},
		{"cov.jpg", "COVDATA"},
	})

	meta, err := extractEPUB(path)
	if err != nil {
		t.Fatal(err)
	}
	if meta.ISBN10 != "0306406152" {
		t.Errorf("isbn10 = %q", meta.ISBN10)
	}
	if string(meta.CoverData) != "COVDATA" {
		t.Errorf("cover = %q, want COVDATA via meta name=cover indirection", meta.CoverData)
	}
}

func TestExtractEPUBNoCoverGuess(t *testing.T) {
	// no cover-image property, no meta cover, no "cover" in names:
	// better no cover (providers backfill) than a random image
	opf := `<?xml version="1.0"?>
<package xmlns="http://www.idpf.org/2007/opf">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title>Plain</dc:title></metadata>
  <manifest><item id="i1" href="photo.png" media-type="image/png"/></manifest>
</package>`
	path := writeEPUB(t, [][2]string{
		{"META-INF/container.xml", `<container xmlns="urn:oasis:names:tc:opendocument:xmlns:container"><rootfiles><rootfile full-path="content.opf"/></rootfiles></container>`},
		{"content.opf", opf},
		{"photo.png", "PHOTO"},
	})
	meta, err := extractEPUB(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(meta.CoverData) != 0 {
		t.Errorf("cover = %q, want none", meta.CoverData)
	}
}

func TestISBNValidation(t *testing.T) {
	cases := []struct {
		in      string
		v13, v10 bool
	}{
		{"9780134685991", true, false},
		{"9780306406157", true, false},
		{"9780134685990", false, false}, // bad check digit
		{"1234567890123", false, false}, // not 978/979
		{"0306406152", false, true},
		{"097522980X", false, true},
		{"1012345678", false, false}, // DOI-shaped digit run
		{"0306406153", false, false}, // bad check digit
	}
	for _, c := range cases {
		if got := isValidISBN13(c.in); got != c.v13 {
			t.Errorf("isValidISBN13(%q) = %v", c.in, got)
		}
		if got := isValidISBN10(c.in); got != c.v10 {
			t.Errorf("isValidISBN10(%q) = %v", c.in, got)
		}
	}
}

func TestContentText(t *testing.T) {
	cases := []struct{ in, want string }{
		// TJ kerning must rejoin without separators
		{`[(97)-8(8-0)] TJ`, "978-0"},
		// separate text runs must not merge into one token
		{`(ISBN) Tj 1 0 0 1 72 700 Tm (978-0-306-40615-7) Tj`, "ISBN 978-0-306-40615-7"},
		{`(par\(en\)s and \\slash) Tj`, `par(en)s and \slash`},
		{`(oct\101l) Tj`, "octAl"},
	}
	for _, c := range cases {
		if got := contentText([]byte(c.in)); got != c.want {
			t.Errorf("contentText(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// writePDF builds a minimal single-page classic-xref PDF with an Info dict
// and an uncompressed content stream, with correct byte offsets.
func writePDF(t *testing.T, content string) string {
	t.Helper()
	objs := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources << /Font << /F1 6 0 R >> >> >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content),
		"<< /Title (Test Driven Development) /Author (Kent Beck; Jane Doe) /Subject (A book about tests) /Keywords (testing, go) >>",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	}
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objs)+1)
	for i, o := range objs {
		offsets[i+1] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	xref := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)
	for i := 1; i <= len(objs); i++ {
		fmt.Fprintf(&buf, "%010d 00000 n \n", offsets[i])
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root 1 0 R /Info 5 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objs)+1, xref)

	path := filepath.Join(t.TempDir(), "book.pdf")
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestExtractPDF(t *testing.T) {
	path := writePDF(t, "BT /F1 12 Tf 72 700 Td (ISBN 978-0-306-40615-7) Tj ET")
	meta, err := extractPDF(path)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Title != "Test Driven Development" {
		t.Errorf("title = %q", meta.Title)
	}
	if !reflect.DeepEqual(meta.Authors, []string{"Kent Beck", "Jane Doe"}) {
		t.Errorf("authors = %v", meta.Authors)
	}
	if meta.Description != "A book about tests" {
		t.Errorf("description = %q", meta.Description)
	}
	if meta.PageCount != 1 {
		t.Errorf("page count = %d", meta.PageCount)
	}
	if !reflect.DeepEqual(meta.Categories, []string{"go", "testing"}) {
		t.Errorf("categories = %v", meta.Categories)
	}
	if meta.ISBN13 != "9780306406157" {
		t.Errorf("isbn13 = %q, want ISBN found in page content", meta.ISBN13)
	}
}

func TestCleanPDFTitle(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Real Title", "Real Title"},
		{"Microsoft Word - final_v3.docx", ""},
		{"chapter1.pdf", ""},
		{"untitled", ""},
		{"Untitled document 2", ""},
		{"  spaced  ", "spaced"},
	}
	for _, c := range cases {
		if got := cleanPDFTitle(c.in); got != c.want {
			t.Errorf("cleanPDFTitle(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
