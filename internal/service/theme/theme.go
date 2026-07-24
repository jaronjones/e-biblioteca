package theme

// Names mirrors DaisyUI theme set used in web/static/css/themes.css
var Names = []string{
	"light", "dark", "cupcake", "bumblebee", "emerald", "corporate",
	"synthwave", "retro", "cyberpunk", "valentine", "halloween", "garden",
	"forest", "aqua", "lofi", "pastel", "fantasy", "wireframe",
	"black", "luxury", "dracula", "cmyk", "autumn", "business",
	"acid", "lemonade", "night", "coffee", "winter", "dim",
	"nord", "sunset", "caramellatte", "abyss", "silk",
}

const Default = "dark"

func Valid(name string) bool {
	for _, n := range Names {
		if n == name {
			return true
		}
	}
	return false
}

func Normalize(name string) string {
	if Valid(name) {
		return name
	}
	return Default
}
