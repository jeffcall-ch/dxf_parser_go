package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"runtime"
	"strconv"
	"time"
)

// CLI handles command-line interface operations
func runCLI() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]
	
	switch command {
	case "parse":
		handleParseCommand()
	case "spatial":
		handleSpatialCommand()
	case "benchmark":
		handleBenchmarkCommand()
	case "bom":
		bomMain()
	case "help":
		printUsage()
	default:
		fmt.Printf("Unknown command: %s\n\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("DXF Text Parser - High-performance text extraction from DXF files")
	fmt.Println("\nUsage:")
	fmt.Println("  dxf_parser parse <file.dxf> [workers]     - Parse DXF file and show results")
	fmt.Println("  dxf_parser spatial <file.dxf> [command]  - Run spatial analysis")
	fmt.Println("  dxf_parser benchmark <file.dxf>          - Run performance benchmarks")
	fmt.Println("  dxf_parser bom -dir <directory> [options] - Extract BOM and cut lengths")
	fmt.Println("  dxf_parser help                          - Show this help message")
	fmt.Println("\nSpatial Commands:")
	fmt.Println("  stats                                    - Show entity statistics")
	fmt.Println("  near <text> <distance>                  - Find entities near text")
	fmt.Println("  range <minX> <minY> <maxX> <maxY>       - Find entities in coordinate range")
	fmt.Println("  quadrant <text>                         - Find entities in top-right quadrant of text")
	fmt.Println("\nExamples:")
	fmt.Println("  dxf_parser parse drawing.dxf 8")
	fmt.Println("  dxf_parser spatial drawing.dxf stats")
	fmt.Println("  dxf_parser spatial drawing.dxf near \"PIPE\" 50.0")
	fmt.Println("  dxf_parser benchmark drawing.dxf")
}

func handleParseCommand() {
	if len(os.Args) < 3 {
		fmt.Println("Error: Missing DXF file argument")
		fmt.Println("Usage: dxf_parser parse <file.dxf> [workers]")
		os.Exit(1)
	}

	filename := os.Args[2]
	workers := runtime.NumCPU()

	if len(os.Args) > 3 {
		if w, err := strconv.Atoi(os.Args[3]); err == nil && w > 0 {
			workers = w
		}
	}

	fmt.Printf("Parsing DXF file: %s\n", filename)
	fmt.Printf("Using %d workers\n", workers)

	parser := NewDXFParser(workers)
	
	start := time.Now()
	entities, err := parser.ParseFile(filename)
	duration := time.Since(start)

	if err != nil {
		log.Fatalf("Error parsing file: %v", err)
	}

	fmt.Printf("\nParsing completed in: %v\n", duration)
	fmt.Printf("Found %d text entities\n\n", len(entities))

	// Display first 10 entities
	limit := 10
	if len(entities) < limit {
		limit = len(entities)
	}

	fmt.Printf("First %d text entities:\n", limit)
	fmt.Println("----------------------------------------")
	for i := 0; i < limit; i++ {
		entity := entities[i]
		fmt.Printf("%d. %s: \"%s\" at (%.3f, %.3f) height=%.2f layer=%s\n",
			i+1, entity.EntityType, entity.Content, entity.X, entity.Y, entity.Height, entity.Layer)
	}

	if len(entities) > limit {
		fmt.Printf("... and %d more entities\n", len(entities)-limit)
	}
}

func handleSpatialCommand() {
	if len(os.Args) < 4 {
		fmt.Println("Error: Missing arguments for spatial command")
		fmt.Println("Usage: dxf_parser spatial <file.dxf> <command> [args...]")
		os.Exit(1)
	}

	filename := os.Args[2]
	spatialCmd := os.Args[3]

	// Parse the file
	parser := NewDXFParser(runtime.NumCPU())
	entities, err := parser.ParseFile(filename)
	if err != nil {
		log.Fatalf("Error parsing file: %v", err)
	}

	analyzer := NewSpatialAnalyzer(entities)

	switch spatialCmd {
	case "stats":
		handleStatsCommand(analyzer)
	case "near":
		handleNearCommand(analyzer)
	case "range":
		handleRangeCommand(analyzer)
	case "quadrant":
		handleQuadrantCommand(analyzer)
	default:
		fmt.Printf("Unknown spatial command: %s\n", spatialCmd)
		os.Exit(1)
	}
}

func handleStatsCommand(analyzer *SpatialAnalyzer) {
	stats := analyzer.GetEntityStats()
	
	fmt.Println("DXF File Statistics:")
	fmt.Println("===================")
	
	statsJSON, _ := json.MarshalIndent(stats, "", "  ")
	fmt.Println(string(statsJSON))
}

func handleNearCommand(analyzer *SpatialAnalyzer) {
	if len(os.Args) < 6 {
		fmt.Println("Usage: dxf_parser spatial <file.dxf> near <text> <distance>")
		os.Exit(1)
	}

	searchText := os.Args[4]
	distance, err := strconv.ParseFloat(os.Args[5], 64)
	if err != nil {
		fmt.Printf("Error: Invalid distance value: %s\n", os.Args[5])
		os.Exit(1)
	}

	fmt.Printf("Finding entities near \"%s\" within distance %.2f:\n\n", searchText, distance)
	
	nearEntities := analyzer.FindEntitiesNearText(searchText, distance)
	
	if len(nearEntities) == 0 {
		fmt.Println("No entities found near the specified text.")
		return
	}

	fmt.Printf("Found %d entities:\n", len(nearEntities))
	fmt.Println("----------------------------------------")
	
	for i, entityWithDistance := range nearEntities {
		entity := entityWithDistance.Entity
		fmt.Printf("%d. \"%s\" at (%.3f, %.3f) - distance: %.3f\n",
			i+1, entity.Content, entity.X, entity.Y, entityWithDistance.Distance)
	}
}

func handleRangeCommand(analyzer *SpatialAnalyzer) {
	if len(os.Args) < 8 {
		fmt.Println("Usage: dxf_parser spatial <file.dxf> range <minX> <minY> <maxX> <maxY>")
		os.Exit(1)
	}

	minX, err1 := strconv.ParseFloat(os.Args[4], 64)
	minY, err2 := strconv.ParseFloat(os.Args[5], 64)
	maxX, err3 := strconv.ParseFloat(os.Args[6], 64)
	maxY, err4 := strconv.ParseFloat(os.Args[7], 64)

	if err1 != nil || err2 != nil || err3 != nil || err4 != nil {
		fmt.Println("Error: Invalid coordinate values")
		os.Exit(1)
	}

	fmt.Printf("Finding entities in range (%.2f, %.2f) to (%.2f, %.2f):\n\n", minX, minY, maxX, maxY)
	
	entities := analyzer.FindEntitiesInRange(minX, minY, maxX, maxY)
	
	if len(entities) == 0 {
		fmt.Println("No entities found in the specified range.")
		return
	}

	fmt.Printf("Found %d entities:\n", len(entities))
	fmt.Println("----------------------------------------")
	
	for i, entity := range entities {
		fmt.Printf("%d. \"%s\" at (%.3f, %.3f)\n",
			i+1, entity.Content, entity.X, entity.Y)
	}
}

func handleQuadrantCommand(analyzer *SpatialAnalyzer) {
	if len(os.Args) < 5 {
		fmt.Println("Usage: dxf_parser spatial <file.dxf> quadrant <text>")
		os.Exit(1)
	}

	searchText := os.Args[4]
	
	fmt.Printf("Finding entities in top-right quadrant relative to \"%s\":\n\n", searchText)
	
	entities := analyzer.FindEntitiesInTopRightQuadrant(searchText)
	
	if len(entities) == 0 {
		fmt.Println("No entities found in the top-right quadrant of the specified text.")
		return
	}

	fmt.Printf("Found %d entities:\n", len(entities))
	fmt.Println("----------------------------------------")
	
	for i, entity := range entities {
		fmt.Printf("%d. \"%s\" at (%.3f, %.3f)\n",
			i+1, entity.Content, entity.X, entity.Y)
	}
}

func handleBenchmarkCommand() {
	if len(os.Args) < 3 {
		fmt.Println("Error: Missing DXF file argument")
		fmt.Println("Usage: dxf_parser benchmark <file.dxf>")
		os.Exit(1)
	}

	filename := os.Args[2]
	
	fmt.Printf("Running benchmarks on: %s\n", filename)
	fmt.Println("=====================================")

	// Test different worker counts
	workerCounts := []int{1, 2, 4, 8, runtime.NumCPU()}
	
	var baselineTime time.Duration
	var baselineEntities int

	for i, workers := range workerCounts {
		if workers > runtime.NumCPU() {
			continue
		}

		fmt.Printf("\nBenchmark %d: %d workers\n", i+1, workers)
		fmt.Println("-------------------------")

		parser := NewDXFParser(workers)
		
		// Run multiple iterations for average
		iterations := 3
		var totalTime time.Duration
		var entityCount int

		for j := 0; j < iterations; j++ {
			start := time.Now()
			entities, err := parser.ParseFile(filename)
			duration := time.Since(start)

			if err != nil {
				log.Fatalf("Error in benchmark: %v", err)
			}

			totalTime += duration
			entityCount = len(entities)
			
			fmt.Printf("  Run %d: %v (%d entities)\n", j+1, duration, entityCount)
		}

		avgTime := totalTime / time.Duration(iterations)
		fmt.Printf("  Average: %v\n", avgTime)

		if i == 0 {
			baselineTime = avgTime
			baselineEntities = entityCount
		} else {
			speedup := float64(baselineTime) / float64(avgTime)
			fmt.Printf("  Speedup: %.2fx\n", speedup)
		}
	}

	fmt.Printf("\nTotal entities found: %d\n", baselineEntities)
	
	// Memory usage estimate
	entitySize := 120 // Rough estimate of TextEntity struct size in bytes
	memoryUsage := float64(baselineEntities * entitySize) / (1024 * 1024)
	fmt.Printf("Estimated memory usage: %.2f MB\n", memoryUsage)
}
