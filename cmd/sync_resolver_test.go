package cmd

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// TestResolveWikilink_NeverNonMd es la red de seguridad contra el bug que
// metía nodos no-.md en el grafo (gpg-key-humanbyte-net/index.html,
// chathub_arquitectura.drawio, historico_y_reportes/ con "/" final).
//
// Política bajo prueba (post-2026-08-12):
//   - El resolver SOLO retorna paths a archivos .md regulares.
//   - Paths a .html, .drawio, directorios u otros → "" (wikilink roto).
//
// Layout del tmpdir:
//   tmp/
//     a.md                           ← destino .md válido
//     sub/
//       b.md                         ← destino .md válido (dir(from) prueba)
//       sub/index.md                 ← destino .md válido (index.md prueba)
//     index.html                     ← trampa: .html, NO debe resolver
//     archivo.drawio                 ← trampa: .drawio, NO debe resolver
//     directorio_trampa/             ← trampa: dir sin .md, NO debe resolver
//       algo.md                      ← trampa: md adentro de dir trampa
func TestResolveWikilink_NeverNonMd(t *testing.T) {
	vault := t.TempDir()

	// Layout base.
	must := func(rel, content string) {
		dir := filepath.Dir(filepath.Join(vault, rel))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(vault, rel), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	must("a.md", "x")
	must("sub/b.md", "x")
	must("sub/sub/index.md", "x")
	// Trampas no-.md que el resolver debe IGNORAR.
	must("index.html", "<html></html>")
	must("archivo.drawio", "drawio")
	must("directorio_trampa/algo.md", "x") // existe el .md pero el padre es trampa de link

	// from arbitrario en la raíz del vault; el resolver no requiere que
	// el archivo exista, solo necesita Dir(fromPath) para los candidatos.
	fromPath := "nota_origen.md"

	cases := []struct {
		name string
		link string
		want string
	}{
		// Casos válidos (deben resolver).
		{"md-en-raiz", "a", "a.md"},
		{"md-en-subdir", "sub/b", "sub/b.md"},
		{"index-md-en-subdir-doble", "sub/sub", "sub/sub/index.md"},

		// Casos trampa: el resolver NO debe aceptar estos aunque el
		// path exista en disco.
		{"html-en-raiz", "index", ""},                 // existe index.html, no index.md
		{"drawio-en-raiz", "archivo", ""},             // existe archivo.drawio, no archivo.md
		{"directorio-sin-md", "directorio_trampa", ""}, // dir existe pero no tiene .md

		// Links a nombres que no existen en absoluto.
		{"no-existe", "fantasma_inexistente", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveWikilink(vault, fromPath, tc.link)
			if got != tc.want {
				t.Fatalf("resolveWikilink(%q) = %q, want %q", tc.link, got, tc.want)
			}
		})
	}
}

// TestResolveWikilink_AcceptsOnlyRegularFiles confirma que un path
// candidato que existe pero es directorio se ignora aunque termine en .md.
//
// Esto cubre el caso del bug original:
//   wikilink: [[Planes/ChatHub/historico_y_reportes/]]
//   candidato: Planes/ChatHub/historico_y_reportes.md (NO existe)
//   fallback a "sin extensión": Plans/ChatHub/historico_y_reportes (existe COMO DIR)
//   bug: el resolver devolvía "Planes/ChatHub/historico_y_reportes/" (con /).
//
// Bajo la política nueva, el resolver retorna "" para este caso aunque
// exista el path en disco, porque es directorio y/o no termina en .md.
func TestResolveWikilink_AcceptsOnlyRegularFiles(t *testing.T) {
	vault := t.TempDir()

	// Crear un archivo con nombre que termina en ".md" pero que en
	// realidad es un directorio. Esto valida el filtro info.IsDir().
	mdAsDir := filepath.Join(vault, "trampa_dir.md")
	if err := os.Mkdir(mdAsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	fromPath := "x.md"
	got := resolveWikilink(vault, fromPath, "trampa_dir")
	if got != "" {
		t.Fatalf("resolveWikilink debe rechazar directorio con sufijo .md, got %q", got)
	}
}

// TestCollectMarkdownFiles_NoDirsWithMdSuffix confirma que
// collectMarkdownFiles sigue el filtro del walker (excluye directorios y
// solo emite paths a archivos con extensión .md). Esta era la línea base
// antes del fix; la regresión a vigilar es que files termine conteniendo
// strings como "Planes/ChatHub/historico_y_reportes/" (path de directorio).
func TestCollectMarkdownFiles_NoDirsWithMdSuffix(t *testing.T) {
	vault := t.TempDir()

	mkdir := func(rel string) {
		if err := os.MkdirAll(filepath.Join(vault, rel), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	mkfile := func(rel, body string) {
		mkdir(filepath.Dir(rel))
		if err := os.WriteFile(filepath.Join(vault, rel), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Caso base: archivos .md normales se incluyen.
	mkfile("a.md", "x")
	mkfile("sub/b.md", "x")

	// Trampas: directorios con extensión .md deben excluirse, igual que
	// archivos no-.md.
	mkdir("trampa_dir.md")
	mkfile("trampa_html.html", "<html></html>")
	mkfile("trampa_drawio.drawio", "x")

	paths, err := collectMarkdownFiles(vault)
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(paths)

	want := []string{"a.md", "sub/b.md"}
	if len(paths) != len(want) {
		t.Fatalf("paths=%v, want=%v", paths, want)
	}
	for i, p := range paths {
		if p != want[i] {
			t.Fatalf("paths[%d]=%q, want %q", i, p, want[i])
		}
	}
}
