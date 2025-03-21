package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/dontizi/rlama/internal/domain"
	"github.com/dontizi/rlama/internal/service"
	"github.com/spf13/cobra"
)

var (
	contextSize int
	showContext bool
)

var runCmd = &cobra.Command{
	Use:   "run [rag-name]",
	Short: "Run a RAG system",
	Long: `Run a previously created RAG system. 
Starts an interactive session to interact with the RAG system.
Example: rlama run rag1`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ragName := args[0]

		// Get Ollama client with configured host and port
		ollamaClient := GetOllamaClient()
		if err := ollamaClient.CheckOllamaAndModel(""); err != nil {
			return err
		}

		ragService := service.NewRagService(ollamaClient)
		rag, err := ragService.LoadRag(ragName)
		if err != nil {
			return err
		}

		fmt.Printf("RAG '%s' loaded. Model: %s\n", rag.Name, rag.ModelName)
		if showContext {
			fmt.Printf("Debug info: RAG contains %d documents and %d total chunks\n",
				len(rag.Documents), len(rag.Chunks))
			fmt.Printf("Chunking strategy: %s, Size: %d, Overlap: %d\n",
				rag.ChunkingStrategy,
				rag.WatchOptions.ChunkSize,
				rag.WatchOptions.ChunkOverlap)
		}
		fmt.Println("Type your question (or 'exit' to quit):")

		scanner := bufio.NewScanner(os.Stdin)
		for {
			fmt.Print("> ")
			if !scanner.Scan() {
				break
			}

			question := scanner.Text()
			if question == "exit" {
				break
			}

			if strings.TrimSpace(question) == "" {
				continue
			}

			fmt.Println("\nSearching documents for relevant information...")

			checkWatchedResources(rag, ragService)

			// If debug mode is enabled, get the chunks manually first
			if showContext {
				// Call embeddingService directly through ragService to generate embedding
				embeddingService := service.NewEmbeddingService(ollamaClient)
				queryEmbedding, err := embeddingService.GenerateQueryEmbedding(question, rag.ModelName)
				if err != nil {
					fmt.Printf("Error generating embedding: %s\n", err)
				} else {
					results := rag.HybridStore.Search(queryEmbedding, contextSize)

					// Show detailed results
					fmt.Printf("\n--- Debug: Retrieved %d chunks ---\n", len(results))
					for i, result := range results {
						chunk := rag.GetChunkByID(result.ID)
						if chunk != nil {
							fmt.Printf("%d. [Score: %.4f] %s\n", i+1, result.Score, chunk.GetMetadataString())
							if i < 3 { // Show content for top 3 chunks only to avoid overload
								fmt.Printf("   Preview: %s\n", truncateString(chunk.Content, 100))
							}
						}
					}
					fmt.Println("--- End Debug ---\n")
				}
			}

			answer, err := ragService.Query(rag, question, contextSize)
			if err != nil {
				fmt.Printf("Error: %s\n", err)
				continue
			}

			fmt.Println("\n--- Answer ---")
			fmt.Println(answer)
			fmt.Println()
		}

		return nil
	},
}

// Helper function to truncate string for preview
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func init() {
	rootCmd.AddCommand(runCmd)

	// Add flags
	runCmd.Flags().IntVar(&contextSize, "context-size", 20, "Number of context chunks to retrieve (default: 20)")
	runCmd.Flags().BoolVar(&showContext, "show-context", false, "Show retrieved chunks and context information")
}

func checkWatchedResources(rag *domain.RagSystem, ragService service.RagService) {
	// Check watched directory if enabled with on-use check
	if rag.WatchEnabled && rag.WatchInterval == 0 {
		fileWatcher := service.NewFileWatcher(ragService)
		docsAdded, err := fileWatcher.CheckAndUpdateRag(rag)
		if err != nil {
			fmt.Printf("Error checking watched directory: %v\n", err)
		} else if docsAdded > 0 {
			fmt.Printf("Added %d new documents from watched directory.\n", docsAdded)
		}
	}

	// Check watched website if enabled with on-use check
	if rag.WebWatchEnabled && rag.WebWatchInterval == 0 {
		webWatcher := service.NewWebWatcher(ragService)
		pagesAdded, err := webWatcher.CheckAndUpdateRag(rag)
		if err != nil {
			fmt.Printf("Error checking watched website: %v\n", err)
		} else if pagesAdded > 0 {
			fmt.Printf("Added %d new pages from watched website.\n", pagesAdded)
		}
	}
}
