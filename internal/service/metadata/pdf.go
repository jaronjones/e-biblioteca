package metadata

import (
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"

	"github.com/jjones/e-biblioteca/internal/models"
)

const isbnScanMaxPages = 10

// extractPDF reads the document information dictionary and page count, then
// scans the first pages' content streams for an ISBN (usually on the
// copyright page).
func extractPDF(path string) (models.ExtractedMetadata, error) {
	var meta models.ExtractedMetadata
	f, err := os.Open(path)
	if err != nil {
		return meta, err
	}
	defer f.Close()

	conf := model.NewDefaultConfiguration()
	conf.ValidationMode = model.ValidationRelaxed
	conf.Cmd = model.LISTINFO
	ctx, err := api.ReadAndValidate(f, conf)
	if err != nil {
		return meta, err
	}

	meta.Title = cleanPDFTitle(ctx.Title)
	if a := strings.TrimSpace(ctx.Author); a != "" {
		meta.Authors = splitAuthors(a)
	}
	meta.Description = strings.TrimSpace(ctx.Subject)
	meta.PageCount = ctx.PageCount
	if kw, err := pdfcpu.KeywordsList(ctx); err == nil {
		meta.Categories = cleanList(kw)
	}

	meta.ISBN13, meta.ISBN10 = scanPDFISBN(ctx, isbnScanMaxPages)
	return meta, nil
}

// Producers often stuff source filenames or placeholders into /Title;
// an empty title falls back to the humanized filename in Extract.
var junkPDFTitle = regexp.MustCompile(`(?i)^(untitled( document)?[0-9 ]*|microsoft \w+ - .*|.*\.(docx?|rtf|indd|qxd|tex|dvi|pptx?|odt|pages|pdf))$`)

func cleanPDFTitle(s string) string {
	s = strings.TrimSpace(s)
	if junkPDFTitle.MatchString(s) {
		return ""
	}
	return s
}

var isbnRe = regexp.MustCompile(`(?i)ISBN(?:-1[03])?[:\s]*([0-9][0-9 \-]{8,20}[0-9Xx])`)

func scanPDFISBN(ctx *model.Context, maxPages int) (isbn13, isbn10 string) {
	// pdfcpu's page traversal can panic on damaged files
	defer func() { _ = recover() }()
	n := ctx.PageCount
	if n > maxPages {
		n = maxPages
	}
	for pageNr := 1; pageNr <= n; pageNr++ {
		r, err := pdfcpu.ExtractPageContent(ctx, pageNr)
		if err != nil || r == nil {
			continue
		}
		b, err := io.ReadAll(io.LimitReader(r, 2<<20))
		if err != nil {
			continue
		}
		for _, m := range isbnRe.FindAllStringSubmatch(contentText(b), -1) {
			cleaned := strings.ToUpper(onlyDigits(m[1]))
			if isbn13 == "" && isValidISBN13(cleaned) {
				isbn13 = cleaned
			}
			if isbn10 == "" && isValidISBN10(cleaned) {
				isbn10 = cleaned
			}
		}
		if isbn13 != "" && isbn10 != "" {
			return
		}
	}
	return
}

// contentText pulls text-show string literals out of a decoded content
// stream. Literals split by TJ kerning offsets rejoin with no separator
// ("[(97)-8(8-0)]" -> "978-0"); literals separated by operators get a
// space so distinct text runs don't merge into one token.
func contentText(b []byte) string {
	var sb strings.Builder
	i := 0
	sawOperator := false
	for i < len(b) {
		c := b[i]
		if c == '(' {
			if sawOperator {
				sb.WriteByte(' ')
			}
			sawOperator = false
			i++
			depth := 1
			for i < len(b) && depth > 0 {
				switch b[i] {
				case '\\':
					if i+1 < len(b) {
						i++
						switch e := b[i]; e {
						case 'n', 'r', 't', 'b', 'f':
							sb.WriteByte(' ')
						case '(', ')', '\\':
							sb.WriteByte(e)
						default:
							if e >= '0' && e <= '7' {
								// octal escape: consume up to 3 digits
								v := int(e - '0')
								for k := 0; k < 2 && i+1 < len(b) && b[i+1] >= '0' && b[i+1] <= '7'; k++ {
									i++
									v = v*8 + int(b[i]-'0')
								}
								if v >= 0x20 && v < 0x7f {
									sb.WriteByte(byte(v))
								}
							} else {
								sb.WriteByte(e)
							}
						}
					}
				case '(':
					depth++
					sb.WriteByte('(')
				case ')':
					depth--
					if depth > 0 {
						sb.WriteByte(')')
					}
				default:
					sb.WriteByte(b[i])
				}
				i++
			}
			continue
		}
		if c >= 'A' && c <= 'z' && (c <= 'Z' || c >= 'a') {
			sawOperator = true
		}
		i++
	}
	return sb.String()
}

func isValidISBN13(s string) bool {
	if len(s) != 13 || (!strings.HasPrefix(s, "978") && !strings.HasPrefix(s, "979")) {
		return false
	}
	sum := 0
	for i := 0; i < 13; i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return false
		}
		d := int(c - '0')
		if i%2 == 1 {
			d *= 3
		}
		sum += d
	}
	return sum%10 == 0
}

func isValidISBN10(s string) bool {
	if len(s) != 10 {
		return false
	}
	sum := 0
	for i := 0; i < 10; i++ {
		c := s[i]
		var d int
		switch {
		case c >= '0' && c <= '9':
			d = int(c - '0')
		case c == 'X' && i == 9:
			d = 10
		default:
			return false
		}
		sum += (10 - i) * d
	}
	return sum%11 == 0
}
