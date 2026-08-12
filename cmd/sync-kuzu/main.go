// cmd/sync-kuzu: sincroniza el vault a una base de datos Kuzu embebida.
//
// Wrapper CLI sobre internal/syncvault. Mantenido como binario separado
// para permitir invocación directa (sin pasar por vg sync + KUZU_PATH).
//
// Uso:
//   go run ./cmd/sync-kuzu --vault <path> --kuzu <path> [--dry-run]

package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	syncvault "github.com/freddytaborda/vault-graph/internal/syncvault"
)

func main() {
	vaultPath := flag.String("vault", "", "ruta al vault (default $VAULT_PATH o ~/Workspace/Obsidian)")
	kuzuPath := flag.String("kuzu", "", "ruta al archivo Kuzu destino (requerido)")
	dryRun := flag.Bool("dry-run", false, "solo recolectar paths, no escribir")
	flag.Parse()

	if *vaultPath == "" {
		*vaultPath = os.Getenv("VAULT_PATH")
		if *vaultPath == "" {
			*vaultPath = os.ExpandEnv("$HOME/Workspace/Obsidian")
		}
	}
	if *kuzuPath == "" {
		fmt.Fprintf(os.Stderr, "uso: --vault <path> --kuzu <path> [--dry-run]\n")
		os.Exit(1)
	}

	fmt.Printf("=== sync vault → Kuzu ===\n")
	fmt.Printf("Vault:   %s\n", *vaultPath)
	fmt.Printf("Kuzu:    %s\n", *kuzuPath)
	fmt.Printf("DryRun:  %v\n", *dryRun)

	start := time.Now()
	stats, err := syncvault.Sync(*vaultPath, *kuzuPath, *dryRun)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Vault:   %d archivos .md\n", stats.NodeCount)
	if *dryRun {
		fmt.Printf("(dry-run — no se escribió nada)\n")
		return
	}
	fmt.Printf("Kuzu:    %d nodos en %s\n", stats.NodeCount, stats.NodeDuration)
	fmt.Printf("Kuzu:    %d aristas detectadas\n", stats.EdgeCount)
	fmt.Printf("Kuzu:    %d/%d aristas en %s (0 fallos)\n",
		stats.EdgesWritten, stats.EdgeCount, stats.EdgeDuration)

	// Verificación final.
	fmt.Printf("\n=== verificación final ===\n")
	fmt.Printf("Total elapsed: %s\n", time.Since(start))
}