// Test de integración para migrate-age-to-kuzu.
//
// Valida:
//   1. dry-run contra AGE real: cuenta nodos, no escribe.
//   2. Escritura con --limit 3 a archivo temporal: 3 nodos, 0 aristas.
//   3. El archivo Kuzu resultante abre y tiene los conteos esperados.
//
// Por qué --limit 3: para no mover los 1,409 nodos del vault, solo unos
// cuantos de prueba. El test es read-only sobre AGE y write sobre
// un archivo temporal que se borra al final.

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMigrateDryRun(t *testing.T) {
	ageURL := os.Getenv("DATABASE_URL")
	if ageURL == "" {
		t.Skip("DATABASE_URL no configurada; saltando test de integración con AGE real")
	}

	tmpDir := t.TempDir()
	kuzuPath := filepath.Join(tmpDir, "dryrun.kuzu")

	stats, err := migrate(ageURL, kuzuPath, 0, true)
	if err != nil {
		t.Fatalf("migrate dry-run: %v", err)
	}

	// Validar conteos de AGE (esperamos 1,409+ nodos con label en el vault real).
	if stats.AgeLabeledNodes < 1000 {
		t.Errorf("AGE labeled nodes = %d, esperado >= 1000 (vault real tiene ~1409)",
			stats.AgeLabeledNodes)
	}

	// Dry-run no debe escribir nada.
	if _, err := os.Stat(kuzuPath); err == nil {
		t.Errorf("dry-run creó el archivo %s, no debería", kuzuPath)
	}
}

func TestMigrateWithLimit(t *testing.T) {
	ageURL := os.Getenv("DATABASE_URL")
	if ageURL == "" {
		t.Skip("DATABASE_URL no configurada; saltando test de integración con AGE real")
	}

	tmpDir := t.TempDir()
	kuzuPath := filepath.Join(tmpDir, "pilot.kuzu")

	const limit = 3
	stats, err := migrate(ageURL, kuzuPath, limit, false)
	if err != nil {
		t.Fatalf("migrate con limit %d: %v", limit, err)
	}

	// Esperado: limit nodos migrados.
	if stats.KuzuWrittenNodes != limit {
		t.Errorf("nodos escritos: got %d, esperado %d",
			stats.KuzuWrittenNodes, limit)
	}

	// Sin fallos.
	if stats.KuzuFailedNodes != 0 {
		t.Errorf("nodos fallidos: got %d, esperado 0", stats.KuzuFailedNodes)
	}

	// Verificación final coincide con escritos.
	if stats.KuzuVerifiedNodes != limit {
		t.Errorf("verificación Kuzu: got %d, esperado %d",
			stats.KuzuVerifiedNodes, limit)
	}

	// El archivo existe.
	if _, err := os.Stat(kuzuPath); err != nil {
		t.Errorf("archivo Kuzu no creado: %v", err)
	}
}

func TestMigrateInvalidAgeURL(t *testing.T) {
	// URL inválida debe retornar error, no panic.
	_, err := migrate("postgresql://invalid:invalid@nonexistent:5432/none", "/tmp/nope.kuzu", 0, true)
	if err == nil {
		t.Error("URL inválida debería retornar error")
	}
}

func TestMigrateEmptyAgeURL(t *testing.T) {
	// URL vacía debe retornar error, no panic.
	_, err := migrate("", "/tmp/nope.kuzu", 0, true)
	if err == nil {
		t.Error("URL vacía debería retornar error")
	}
}
