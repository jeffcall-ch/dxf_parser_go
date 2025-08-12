package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

func bomMain() {
	// Parse command line arguments
	var directory string
	var debug bool
	var workers int
	var weldFlag bool

	flag.StringVar(&directory, "dir", "", "Directory containing DXF files (recursively searched)")
	flag.BoolVar(&debug, "debug", false, "Enable detailed debug output")
	flag.IntVar(&workers, "workers", 0, "Number of parallel workers (default: auto-detect based on file count)")
	flag.BoolVar(&weldFlag, "weld", false, "Generate weld detection CSV files (0005_WELD_COUNTS.csv)")
	
	// Custom usage function
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "DXF Isometric BOM Extractor\n\n")
		fmt.Fprintf(os.Stderr, "Usage: %s -dir <directory> [options]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  %s -dir /path/to/dxf/files\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -dir /path/to/dxf/files -debug\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -dir /path/to/dxf/files -workers 4\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -dir /path/to/dxf/files -weld\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -dir /path/to/dxf/files -weld -debug -workers 8\n", os.Args[0])
	}

	flag.Parse()

	if directory == "" {
		fmt.Fprintf(os.Stderr, "Error: Directory is required\n\n")
		flag.Usage()
		os.Exit(1)
	}

	if _, err := os.Stat(directory); os.IsNotExist(err) {
		fmt.Printf("Error: Directory '%s' does not exist\n", directory)
		os.Exit(1)
	}

	runBOMExtraction(directory, debug, workers, weldFlag)
}

func runBOMExtraction(directory string, debug bool, workers int, weldFlag bool) {
	// Set global debug mode
	debugMode = debug

	start := time.Now()

	var materialRows [][]string
	var cutRows [][]string
	var matHeader []string
	var cutHeader []string
	var summary []SummaryRow

	// Count DXF files first
	dxfFiles := []string{}
	err := filepath.Walk(directory, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && (filepath.Ext(strings.ToLower(path)) == ".dxf") {
			dxfFiles = append(dxfFiles, path)
		}
		return nil
	})

	if err != nil {
		fmt.Printf("Error scanning directory: %v\n", err)
		os.Exit(1)
	}

	totalFiles := len(dxfFiles)
	debugPrint(fmt.Sprintf("[DEBUG] Found %d DXF files to process", totalFiles))

	if totalFiles == 0 {
		fmt.Println("No DXF files found.")
		return
	}

	// Determine if we should use parallel processing
	if workers == 0 {
		// Auto-determine: use parallel processing for multiple files
		if totalFiles > 1 {
			workers = min(totalFiles, runtime.NumCPU())
		} else {
			workers = 1
		}
	}

	var results []DXFResult
	var globalFileCache map[string]FileCache
	
	// Initialize caching if weld flag is enabled
	if weldFlag {
		globalFileCache = make(map[string]FileCache)
	}
	
	if workers > 1 {
		fmt.Printf("Processing %d DXF files using %d parallel workers", totalFiles, workers)
		if weldFlag {
			fmt.Printf(" (with weld caching)")
		}
		fmt.Printf("...\n")
		results, globalFileCache = processFilesParallelWithCaching(dxfFiles, workers, debug, weldFlag)
	} else {
		fmt.Printf("Processing %d DXF files sequentially", totalFiles)
		if weldFlag {
			fmt.Printf(" (with weld caching)")
		}
		fmt.Printf("...\n")
		results, globalFileCache = processFilesSequentialWithCaching(dxfFiles, debug, weldFlag)
	}

	// Aggregate results
	successfulFiles := 0
	totalProcessingTime := 0.0

	for _, result := range results {
		if len(result.MatRows) > 0 {
			if len(matHeader) == 0 {
				matHeader = result.MatHeader
			}
			materialRows = append(materialRows, result.MatRows...)
		}

		if len(result.CutRows) > 0 {
			if len(cutHeader) == 0 {
				cutHeader = result.CutHeader
			}
			cutRows = append(cutRows, result.CutRows...)
		}

		summaryRow := SummaryRow{
			FilePath:       result.FilePath,
			Filename:       result.Filename,
			DrawingNo:      result.DrawingNo,
			PipeClass:      result.PipeClass,
			MatRows:        len(result.MatRows),
			CutRows:        len(result.CutRows),
			MatMissing:     len(result.MatRows) == 0,
			CutMissing:     len(result.CutRows) == 0,
			Error:          result.Error,
			ProcessingTime: result.ProcessingTime,
		}
		summary = append(summary, summaryRow)

		if result.Error == "" {
			successfulFiles++
			totalProcessingTime += result.ProcessingTime
		}
	}

	// Write CSV files
	err = writeOutputFiles(directory, materialRows, cutRows, summary, matHeader, cutHeader)
	if err != nil {
		fmt.Printf("Error writing output files: %v\n", err)
		os.Exit(1)
	}

	// Process weld detection if flag is enabled
	if weldFlag && globalFileCache != nil {
		fmt.Printf("\nProcessing weld detection for %d cached files...\n", len(globalFileCache))
		weldStart := time.Now()
		
		weldResults := processWeldDetection(globalFileCache)
		
		if err := writeWeldCSVs(weldResults, directory); err != nil {
			fmt.Printf("Error writing weld CSV files: %v\n", err)
		} else {
			weldTime := time.Since(weldStart).Seconds()
			fmt.Printf("Weld processing completed in %.3f seconds\n", weldTime)
		}
		
		// Cleanup cache to free memory
		cleanupFileCache(globalFileCache)
	}

	// Final timing summary
	endTime := time.Now()
	totalTime := endTime.Sub(start).Seconds()
	printFinalSummary(totalFiles, successfulFiles, totalTime, totalProcessingTime,
		workers, len(materialRows), len(cutRows), directory)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}


