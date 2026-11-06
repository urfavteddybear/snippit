package paste

// Language is a supported syntax. ID is shared with the frontend editor.
type Language struct {
	ID    string
	Name  string
	Ext   string // file extension used for downloads
	Alias []string
}

// Auto asks the server to detect the language.
const Auto = "auto"

var Languages = []Language{
	{ID: "plaintext", Name: "Plain Text", Ext: "txt", Alias: []string{"text", "txt"}},
	{ID: "javascript", Name: "JavaScript", Ext: "js", Alias: []string{"js"}},
	{ID: "typescript", Name: "TypeScript", Ext: "ts", Alias: []string{"ts"}},
	{ID: "html", Name: "HTML", Ext: "html"},
	{ID: "css", Name: "CSS", Ext: "css"},
	{ID: "json", Name: "JSON", Ext: "json"},
	{ID: "xml", Name: "XML", Ext: "xml"},
	{ID: "python", Name: "Python", Ext: "py", Alias: []string{"py"}},
	{ID: "php", Name: "PHP", Ext: "php"},
	{ID: "go", Name: "Go", Ext: "go", Alias: []string{"golang"}},
	{ID: "rust", Name: "Rust", Ext: "rs", Alias: []string{"rs"}},
	{ID: "java", Name: "Java", Ext: "java"},
	{ID: "c", Name: "C", Ext: "c"},
	{ID: "cpp", Name: "C++", Ext: "cpp", Alias: []string{"c++"}},
	{ID: "csharp", Name: "C#", Ext: "cs", Alias: []string{"cs", "c#"}},
	{ID: "bash", Name: "Bash", Ext: "sh", Alias: []string{"sh", "shell"}},
	{ID: "sql", Name: "SQL", Ext: "sql"},
	{ID: "yaml", Name: "YAML", Ext: "yaml", Alias: []string{"yml"}},
	{ID: "markdown", Name: "Markdown", Ext: "md", Alias: []string{"md"}},
	{ID: "dockerfile", Name: "Dockerfile", Ext: "dockerfile", Alias: []string{"docker"}},
	{ID: "nginx", Name: "Nginx", Ext: "conf"},
}

var languageIndex = func() map[string]*Language {
	m := make(map[string]*Language)
	for i := range Languages {
		l := &Languages[i]
		m[l.ID] = l
		for _, a := range l.Alias {
			m[a] = l
		}
	}
	return m
}()

// LookupLanguage resolves an ID or alias to a supported language.
func LookupLanguage(id string) (*Language, bool) {
	l, ok := languageIndex[id]
	return l, ok
}

// LanguageName returns the display name, falling back to Plain Text.
func LanguageName(id string) string {
	if l, ok := languageIndex[id]; ok {
		return l.Name
	}
	return "Plain Text"
}
