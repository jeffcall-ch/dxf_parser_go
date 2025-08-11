package main

import (
	"bufio"
	"encoding/csv"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// PolylineSegment represents a segment of a POLYLINE entity
type PolylineSegment struct {
	X1, Y1, X2, Y2 float64
	Layer          string
	Length         float64
}

// WeldSymbol represents a detected weld symbol (two crossed polyline segments)
type WeldSymbol struct {
	CenterX, CenterY float64
	Length1, Length2 float64
	Layer            string
	Confidence       float64
}

// WeldResult represents weld count for a single file
type WeldResult struct {
	Filename       string `json:"filename"`
	FilePath       string `json:"file_path"`
	WeldCount      int    `json:"weld_count"`
	Error          string `json:"error,omitempty"`
	ProcessingTime float64 `json:"processing_time"`
}

// OptimizedPolylineWeldDetector handles detection with optimized parsing
type OptimizedPolylineWeldDetector struct {
	workers   int
	chunkSize int64
}

func NewOptimizedPolylineWeldDetector(workers int) *OptimizedPolylineWeldDetector {
	if workers <= 0 {
		workers = runtime.NumCPU()
	}
	return &OptimizedPolylineWeldDetector{
		workers:   workers,
		chunkSize: 1024 * 1024,
	}
}

// Known weld symbol length pairs
var weldLengthPairs = [][]float64{
	{4.0311, 6.9462},
	{6.8964, 3.9446},
	{6.9000, 4.0000},
}

// Target lengths for fast filtering
var targetLengths = []float64{4.0311, 6.9462, 6.8964, 3.9446, 6.9000, 4.0000}
var targetLengthsMap map[float64]bool

func init() {
	targetLengthsMap = make(map[float64]bool)
	for _, length := range targetLengths {
		targetLengthsMap[length] = true
	}
}

// distance calculates distance between two points
func distance(x1, y1, x2, y2 float64) float64 {
	dx := x2 - x1
	dy := y2 - y1
	return math.Sqrt(dx*dx + dy*dy)
}

// isTargetLength checks if a length matches any target length (with tolerance)
func isTargetLength(length float64) bool {
	tolerance := 0.01
	for _, target := range targetLengths {
		if math.Abs(length-target) <= tolerance {
			return true
		}
	}
	return false
}

// parsePolylineSegmentsOptimized extracts only target-length POLYLINE segments
func (opwd *OptimizedPolylineWeldDetector) parsePolylineSegmentsOptimized(content string) ([]PolylineSegment, error) {
	var segments []PolylineSegment
	
	scanner := bufio.NewScanner(strings.NewReader(content))
	
	var currentLayer string
	var vertices [][]float64
	inPolyline := false
	inVertex := false
	expectingValue := false
	lastGroupCode := ""
	var currentX, currentY float64

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		
		if !expectingValue {
			lastGroupCode = line
			expectingValue = true
		} else {
			expectingValue = false
			
			switch lastGroupCode {
			case "0": // Entity type
				if line == "POLYLINE" {
					inPolyline = true
					vertices = nil
				} else if line == "SEQEND" && inPolyline {
					// End of POLYLINE, process vertices but only keep target-length segments
					if len(vertices) >= 2 {
						for i := 0; i < len(vertices)-1; i++ {
							segment := PolylineSegment{
								X1:    vertices[i][0],
								Y1:    vertices[i][1],
								X2:    vertices[i+1][0],
								Y2:    vertices[i+1][1],
								Layer: currentLayer,
							}
							segment.Length = distance(segment.X1, segment.Y1, segment.X2, segment.Y2)
							
							// Only keep segments with target lengths
							if isTargetLength(segment.Length) {
								segments = append(segments, segment)
							}
						}
					}
					inPolyline = false
					inVertex = false
				} else if line == "VERTEX" && inPolyline {
					inVertex = true
				}
				
			case "8": // Layer name
				if inPolyline {
					currentLayer = line
				}
				
			case "10": // X coordinate
				if inPolyline && inVertex {
					if val, err := strconv.ParseFloat(line, 64); err == nil {
						currentX = val
					}
				}
				
			case "20": // Y coordinate
				if inPolyline && inVertex {
					if val, err := strconv.ParseFloat(line, 64); err == nil {
						currentY = val
						vertices = append(vertices, []float64{currentX, currentY})
						inVertex = false
					}
				}
			}
		}
	}
	
	return segments, scanner.Err()
}

// lengthsMatch checks if two lengths match any known weld symbol pair
func lengthsMatch(len1, len2 float64) bool {
	tolerance := 0.01 // Allow small floating point variations
	
	for _, pair := range weldLengthPairs {
		// Check both orders: (len1, len2) and (len2, len1)
		if (math.Abs(len1-pair[0]) <= tolerance && math.Abs(len2-pair[1]) <= tolerance) ||
		   (math.Abs(len1-pair[1]) <= tolerance && math.Abs(len2-pair[0]) <= tolerance) {
			return true
		}
	}
	return false
}

// linesIntersect checks if two line segments intersect and returns intersection point
func linesIntersect(seg1, seg2 PolylineSegment) (float64, float64, bool) {
	x1, y1, x2, y2 := seg1.X1, seg1.Y1, seg1.X2, seg1.Y2
	x3, y3, x4, y4 := seg2.X1, seg2.Y1, seg2.X2, seg2.Y2
	
	denom := (x1-x2)*(y3-y4) - (y1-y2)*(x3-x4)
	if math.Abs(denom) < 1e-10 {
		return 0, 0, false // Lines are parallel
	}
	
	t := ((x1-x3)*(y3-y4) - (y1-y3)*(x3-x4)) / denom
	u := -((x1-x2)*(y1-y3) - (y1-y2)*(x1-x3)) / denom
	
	if t >= 0 && t <= 1 && u >= 0 && u <= 1 {
		// Lines intersect
		ix := x1 + t*(x2-x1)
		iy := y1 + t*(y2-y1)
		return ix, iy, true
	}
	
	return 0, 0, false
}

// detectWeldSymbols finds pairs of crossed polyline segments with matching lengths
func (opwd *OptimizedPolylineWeldDetector) detectWeldSymbols(segments []PolylineSegment) []WeldSymbol {
	var weldSymbols []WeldSymbol
	
	fmt.Printf("    Starting detection with %d target segments...\n", len(segments))
	
	if len(segments) == 0 {
		return weldSymbols
	}
	
	// Check all pairs of segments (already filtered to target lengths)
	pairStart := time.Now()
	totalPairs := 0
	validPairs := 0
	intersectionChecks := 0
	
	for i := 0; i < len(segments); i++ {
		for j := i + 1; j < len(segments); j++ {
			totalPairs++
			seg1 := segments[i]
			seg2 := segments[j]
			
			// Check if lengths match known weld symbol pairs
			if !lengthsMatch(seg1.Length, seg2.Length) {
				continue
			}
			validPairs++
			
			// Check if segments intersect (crossed)
			intersectionChecks++
			ix, iy, intersects := linesIntersect(seg1, seg2)
			if !intersects {
				continue
			}
			
			// Check if intersection is roughly in the middle of both segments
			mid1X, mid1Y := (seg1.X1+seg1.X2)/2, (seg1.Y1+seg1.Y2)/2
			mid2X, mid2Y := (seg2.X1+seg2.X2)/2, (seg2.Y1+seg2.Y2)/2
			
			distToMid1 := distance(ix, iy, mid1X, mid1Y)
			distToMid2 := distance(ix, iy, mid2X, mid2Y)
			
			// Intersection should be close to midpoint of both segments
			tolerance1 := seg1.Length * 0.3 // 30% tolerance
			tolerance2 := seg2.Length * 0.3
			
			if distToMid1 > tolerance1 || distToMid2 > tolerance2 {
				continue // Segments don't cross in the middle
			}
			
			// Calculate confidence based on how close to perfect cross it is
			maxTolerance := math.Max(tolerance1, tolerance2)
			maxDistToMid := math.Max(distToMid1, distToMid2)
			confidence := 1.0 - (maxDistToMid / maxTolerance)
			
			// Create weld symbol
			weldSymbol := WeldSymbol{
				CenterX:    ix,
				CenterY:    iy,
				Length1:    seg1.Length,
				Length2:    seg2.Length,
				Layer:      seg1.Layer,
				Confidence: confidence,
			}
			
			weldSymbols = append(weldSymbols, weldSymbol)
		}
	}
	
	pairTime := time.Since(pairStart)
	fmt.Printf("    Pair checking time: %.2f seconds\n", pairTime.Seconds())
	fmt.Printf("    Pair statistics: %d total pairs, %d valid length pairs, %d intersection checks\n", 
		totalPairs, validPairs, intersectionChecks)
	
	// Remove duplicates (same location)
	dedupeStart := time.Now()
	uniqueSymbols := opwd.removeDuplicateSymbols(weldSymbols)
	dedupeTime := time.Since(dedupeStart)
	fmt.Printf("    Deduplication time: %.2f seconds (%d -> %d symbols)\n", 
		dedupeTime.Seconds(), len(weldSymbols), len(uniqueSymbols))
	
	return uniqueSymbols
}

// removeDuplicateSymbols removes weld symbols that are too close to each other
func (opwd *OptimizedPolylineWeldDetector) removeDuplicateSymbols(symbols []WeldSymbol) []WeldSymbol {
	if len(symbols) <= 1 {
		return symbols
	}
	
	var unique []WeldSymbol
	duplicateThreshold := 5.0 // Symbols closer than this are considered duplicates
	
	for _, symbol := range symbols {
		isDuplicate := false
		for _, existing := range unique {
			if distance(symbol.CenterX, symbol.CenterY, existing.CenterX, existing.CenterY) < duplicateThreshold {
				isDuplicate = true
				break
			}
		}
		
		if !isDuplicate {
			unique = append(unique, symbol)
		}
	}
	
	return unique
}

// processFile analyzes a single DXF file for weld symbols
func (opwd *OptimizedPolylineWeldDetector) processFile(filePath string) WeldResult {
	start := time.Now()
	filename := filepath.Base(filePath)
	
	result := WeldResult{
		Filename: filename,
		FilePath: filePath,
	}
	
	// Read file content
	fmt.Printf("Reading file: %s\n", filename)
	readStart := time.Now()
	content, err := os.ReadFile(filePath)
	if err != nil {
		result.Error = fmt.Sprintf("Failed to read file: %v", err)
		result.ProcessingTime = time.Since(start).Seconds()
		return result
	}
	readTime := time.Since(readStart)
	fmt.Printf("  File read time: %.2f seconds (%.1f MB)\n", readTime.Seconds(), float64(len(content))/1024/1024)
	
	// Parse POLYLINE segments (optimized - only target lengths)
	parseStart := time.Now()
	segments, err := opwd.parsePolylineSegmentsOptimized(string(content))
	if err != nil {
		result.Error = fmt.Sprintf("Failed to parse POLYLINE segments: %v", err)
		result.ProcessingTime = time.Since(start).Seconds()
		return result
	}
	parseTime := time.Since(parseStart)
	fmt.Printf("  Optimized parse time: %.2f seconds (%d target segments)\n", parseTime.Seconds(), len(segments))
	
	// Detect weld symbols
	detectStart := time.Now()
	weldSymbols := opwd.detectWeldSymbols(segments)
	detectTime := time.Since(detectStart)
	fmt.Printf("  Detection time: %.2f seconds (%d welds found)\n", detectTime.Seconds(), len(weldSymbols))
	
	result.WeldCount = len(weldSymbols)
	result.ProcessingTime = time.Since(start).Seconds()
	
	totalTime := time.Since(start)
	fmt.Printf("  Total time: %.2f seconds\n", totalTime.Seconds())
	fmt.Printf("  Breakdown: Read %.1f%%, Parse %.1f%%, Detect %.1f%%\n", 
		readTime.Seconds()/totalTime.Seconds()*100,
		parseTime.Seconds()/totalTime.Seconds()*100,
		detectTime.Seconds()/totalTime.Seconds()*100)
	fmt.Println()
	
	return result
}

// processFiles processes multiple DXF files concurrently
func (opwd *OptimizedPolylineWeldDetector) processFiles(filePaths []string) ([]WeldResult, error) {
	resultChan := make(chan WeldResult, len(filePaths))
	
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, opwd.workers)
	
	for _, filePath := range filePaths {
		wg.Add(1)
		go func(fp string) {
			defer wg.Done()
			semaphore <- struct{}{} // Acquire
			
			result := opwd.processFile(fp)
			resultChan <- result
			
			<-semaphore // Release
		}(filePath)
	}
	
	// Close channels when done
	go func() {
		wg.Wait()
		close(resultChan)
	}()
	
	// Collect results
	var results []WeldResult
	for result := range resultChan {
		results = append(results, result)
	}
	
	return results, nil
}

// writeResults writes weld count results to CSV
func writeResults(filename string, results []WeldResult) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()
	
	writer := csv.NewWriter(file)
	defer writer.Flush()
	
	// Write header
	header := []string{"Filename", "FilePath", "WeldCount", "ProcessingTime", "Error"}
	if err := writer.Write(header); err != nil {
		return err
	}
	
	// Write data
	for _, result := range results {
		row := []string{
			result.Filename,
			result.FilePath,
			strconv.Itoa(result.WeldCount),
			fmt.Sprintf("%.3f", result.ProcessingTime),
			result.Error,
		}
		if err := writer.Write(row); err != nil {
			return err
		}
	}
	
	return nil
}

func main() {
	var directory string
	var filePath string
	var workers int
	var output string
	
	flag.StringVar(&directory, "dir", "", "Directory containing DXF files")
	flag.StringVar(&filePath, "file", "", "Single DXF file to analyze")
	flag.IntVar(&workers, "workers", 0, "Number of parallel workers (default: auto)")
	flag.StringVar(&output, "output", "weld_counts.csv", "Output CSV filename")
	
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "DXF Weld Symbol Detector\n\n")
		fmt.Fprintf(os.Stderr, "Detects weld symbols as crossed POLYLINE segments with specific length pairs:\n")
		fmt.Fprintf(os.Stderr, "  - 4.0311 & 6.9462\n")
		fmt.Fprintf(os.Stderr, "  - 6.8964 & 3.9446\n")
		fmt.Fprintf(os.Stderr, "  - 6.9000 & 4.0000\n\n")
		fmt.Fprintf(os.Stderr, "Optimized for maximum speed with parse-time filtering\n\n")
		fmt.Fprintf(os.Stderr, "Usage: %s -dir <directory> [options]\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "       %s -file <dxf_file> [options]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
	}
	
	flag.Parse()
	
	if directory == "" && filePath == "" {
		fmt.Fprintf(os.Stderr, "Error: Either -dir or -file is required\n\n")
		flag.Usage()
		os.Exit(1)
	}
	
	if directory != "" && filePath != "" {
		fmt.Fprintf(os.Stderr, "Error: Cannot specify both -dir and -file\n\n")
		flag.Usage()
		os.Exit(1)
	}
	
	var dxfFiles []string
	
	if filePath != "" {
		// Single file mode
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			fmt.Printf("Error: File '%s' does not exist\n", filePath)
			os.Exit(1)
		}
		dxfFiles = []string{filePath}
	} else {
		// Directory mode
		if _, err := os.Stat(directory); os.IsNotExist(err) {
			fmt.Printf("Error: Directory '%s' does not exist\n", directory)
			os.Exit(1)
		}
		
		// Find DXF files
		err := filepath.Walk(directory, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() && filepath.Ext(strings.ToLower(path)) == ".dxf" {
				dxfFiles = append(dxfFiles, path)
			}
			return nil
		})
		
		if err != nil {
			fmt.Printf("Error scanning directory: %v\n", err)
			os.Exit(1)
		}
	}
	
	if len(dxfFiles) == 0 {
		fmt.Println("No DXF files found.")
		return
	}
	
	fmt.Printf("Found %d DXF files to analyze for POLYLINE weld symbols...\n", len(dxfFiles))
	fmt.Printf("Looking for crossed POLYLINE segments with length pairs:\n")
	for _, pair := range weldLengthPairs {
		fmt.Printf("  - %.4f & %.4f\n", pair[0], pair[1])
	}
	fmt.Printf("Optimized version: Filtering during parse for maximum speed\n")
	fmt.Println()
	
	// Create detector
	detector := NewOptimizedPolylineWeldDetector(workers)
	start := time.Now()
	
	// Process files
	results, err := detector.processFiles(dxfFiles)
	if err != nil {
		fmt.Printf("Error processing files: %v\n", err)
		os.Exit(1)
	}
	
	// Write results
	if err := writeResults(output, results); err != nil {
		fmt.Printf("Error writing results: %v\n", err)
		os.Exit(1)
	}
	
	// Summary
	totalWelds := 0
	successCount := 0
	for _, result := range results {
		if result.Error == "" {
			totalWelds += result.WeldCount
			successCount++
		}
	}
	
	elapsed := time.Since(start)
	
	fmt.Printf("============================================================\n")
	fmt.Printf("WELD SYMBOL DETECTION COMPLETE\n")
	fmt.Printf("============================================================\n")
	fmt.Printf("Total Files: %d\n", len(results))
	fmt.Printf("Successful: %d\n", successCount)
	fmt.Printf("Failed: %d\n", len(results)-successCount)
	fmt.Printf("Total Weld Symbols Found: %d\n", totalWelds)
	if successCount > 0 {
		fmt.Printf("Average Welds per File: %.1f\n", float64(totalWelds)/float64(successCount))
	}
	fmt.Printf("Processing Time: %.2f seconds\n", elapsed.Seconds())
	fmt.Printf("Output File: %s\n", output)
	fmt.Printf("============================================================\n")
}
