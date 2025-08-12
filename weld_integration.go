package main

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Global weld symbol configuration (from original weld_detector.go)
var weldLengthPairs = [][2]float64{
	{4.0311, 6.9462},
	{6.8964, 3.9446},
	{6.9000, 4.0000},
}

var targetLengths = []float64{4.0311, 6.9462, 6.8964, 3.9446, 6.9000, 4.0000}

// distance calculates the distance between two points
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

// FileCache stores parsed data for reuse in weld detection
type FileCache struct {
	TextEntities []TextEntity
	RawContent   []byte
	FilePath     string
	FileName     string
	DrawingNo    string
	PipeClass    string
}

// WeldResult represents the result of weld detection for a single file
type WeldResult struct {
	FilePath       string  `json:"file_path"`
	FileName       string  `json:"file_name"`
	DrawingNo      string  `json:"drawing_no"`
	PipeClass      string  `json:"pipe_class"`
	WeldCount      int     `json:"weld_count"`
	ProcessingTime float64 `json:"processing_time"`
	Error          string  `json:"error"`
}

// WorkerContext holds per-worker cache and results
type WorkerContext struct {
	WorkerID    int
	FileCache   map[string]FileCache
	BOMResults  []DXFResult
	WeldResults []WeldResult
}

// PolylineSegment represents a line segment from polyline parsing
type PolylineSegment struct {
	X1, Y1, X2, Y2 float64
	Length         float64
	Layer          string
}

// WeldSymbol represents a detected weld symbol
type WeldSymbol struct {
	CenterX, CenterY float64
	Length1, Length2 float64
	Layer            string
	Confidence       float64
}

// Performance constants
const (
	MAX_FILES_PER_CHUNK = 300
	MAX_MEMORY_MB       = 4096
)

// createWorkerCaches initializes per-worker cache maps
func createWorkerCaches(numWorkers int) []map[string]FileCache {
	caches := make([]map[string]FileCache, numWorkers)
	for i := range caches {
		caches[i] = make(map[string]FileCache)
	}
	return caches
}

// mergeWorkerCaches combines all worker caches into a single cache
func mergeWorkerCaches(workerCaches []map[string]FileCache) map[string]FileCache {
	globalCache := make(map[string]FileCache)
	
	for _, workerCache := range workerCaches {
		for filePath, fileCache := range workerCache {
			globalCache[filePath] = fileCache
		}
	}
	
	return globalCache
}

// estimateMemoryUsage calculates approximate memory usage of cache
func estimateMemoryUsage(cache map[string]FileCache) int64 {
	var totalBytes int64
	
	for _, fileCache := range cache {
		// Rough estimate: text entities + raw content
		totalBytes += int64(len(fileCache.RawContent))
		totalBytes += int64(len(fileCache.TextEntities) * 200) // rough estimate per TextEntity
	}
	
	return totalBytes / (1024 * 1024) // Convert to MB
}

// cleanupFileCache releases memory from the cache
func cleanupFileCache(cache map[string]FileCache) {
	for filePath := range cache {
		delete(cache, filePath)
	}
}

// processWeldDetection processes cached files for weld detection
func processWeldDetection(fileCache map[string]FileCache) []WeldResult {
	var results []WeldResult
	
	for filePath, cache := range fileCache {
		start := time.Now()
		result := WeldResult{
			FilePath: filePath,
			FileName: cache.FileName,
		}
		
		// Extract drawing number and pipe class from cached text entities
		result.DrawingNo = findDrawingNoFromEntities(cache.TextEntities)
		result.PipeClass = findPipeClassFromEntities(cache.TextEntities)
		
		// Process weld detection safely with error capture
		if weldCount, err := extractWeldsFromRawContent(cache.RawContent); err != nil {
			result.Error = fmt.Sprintf("Weld detection failed: %v", err)
			result.WeldCount = 0
		} else {
			result.WeldCount = weldCount
		}
		
		result.ProcessingTime = time.Since(start).Seconds()
		results = append(results, result)
	}
	
	return results
}

// findDrawingNoFromEntities extracts drawing number from text entities
func findDrawingNoFromEntities(entities []TextEntity) string {
	// First find ERECTION MATERIALS position to establish search area
	var erectionX, erectionY *float64
	for _, entity := range entities {
		if strings.Contains(strings.ToUpper(entity.Content), "ERECTION MATERIALS") {
			erectionX = &entity.X
			erectionY = &entity.Y
			break
		}
	}

	type candidate struct {
		value string
		x     float64
		y     float64
		absY  float64
	}
	candidates := []candidate{}

	for _, entity := range entities {
		match := kksPattern.FindString(entity.Content)
		if match != "" {
			// If we found ERECTION MATERIALS, filter by position (below and to the right)
			if erectionX != nil && erectionY != nil {
				if entity.X >= *erectionX && entity.Y <= *erectionY { // Right and below
					candidates = append(candidates, candidate{match, entity.X, entity.Y, abs(entity.Y)})
				}
			} else {
				// If no ERECTION MATERIALS found, consider all KKS codes
				candidates = append(candidates, candidate{match, entity.X, entity.Y, abs(entity.Y)})
			}
		}
	}

	if len(candidates) > 0 {
		// Sort by Y coordinate (lowest Y = bottom of drawing) and pick the first one
		for i := 0; i < len(candidates)-1; i++ {
			for j := i + 1; j < len(candidates); j++ {
				if candidates[i].absY > candidates[j].absY {
					candidates[i], candidates[j] = candidates[j], candidates[i]
				}
			}
		}
		return candidates[0].value
	}

	return ""
}

// findPipeClassFromEntities extracts pipe class from text entities
func findPipeClassFromEntities(entities []TextEntity) string {
	// Look for 'Pipe class:' label first
	var pipeClassLabelY, pipeClassLabelX *float64

	for _, entity := range entities {
		textClean := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(entity.Content, " ", ""), "\n", ""))
		// More flexible matching for pipe class label
		if strings.Contains(textClean, "pipeclass") ||
			strings.Contains(textClean, "pipe_class") ||
			(strings.Contains(strings.ToLower(entity.Content), "pipe") && strings.Contains(strings.ToLower(entity.Content), "class")) {
			pipeClassLabelX = &entity.X
			pipeClassLabelY = &entity.Y
			break
		}
	}

	if pipeClassLabelY != nil {
		// Look for 4-letter codes near the label
		type candidate struct {
			value    string
			distance float64
		}
		candidates := []candidate{}

		for _, entity := range entities {
			// Look for text near the label (horizontally close, similar Y level)
			if abs(entity.Y-*pipeClassLabelY) < 20 && // Same row or close
				entity.X > *pipeClassLabelX && // To the right of label
				abs(entity.X-*pipeClassLabelX) < 200 { // Not too far horizontally
				textClean := strings.TrimSpace(entity.Content)
				match := pipeClassPattern.FindString(textClean)
				if match != "" {
					distance := abs(entity.X - *pipeClassLabelX)
					candidates = append(candidates, candidate{match, distance})
				}
			}
		}

		if len(candidates) > 0 {
			// Sort by distance from label and pick the closest one
			for i := 0; i < len(candidates)-1; i++ {
				for j := i + 1; j < len(candidates); j++ {
					if candidates[i].distance > candidates[j].distance {
						candidates[i], candidates[j] = candidates[j], candidates[i]
					}
				}
			}
			return candidates[0].value
		}
	}

	// Alternative approach: Look for DESIGN DATA section first, then find pipe class within it
	var designDataY *float64
	for _, entity := range entities {
		if strings.Contains(strings.ToUpper(entity.Content), "DESIGN DATA") {
			designDataY = &entity.Y
			break
		}
	}

	if designDataY != nil {
		// Look for 4-letter codes within DESIGN DATA area (below the title)
		for _, entity := range entities {
			if entity.Y < *designDataY && entity.Y > *designDataY-150 { // Within 150 units below DESIGN DATA
				textClean := strings.TrimSpace(entity.Content)
				match := pipeClassPattern.FindString(textClean)
				if match != "" {
					return match
				}
			}
		}
	}

	// Fallback: look for 4-letter codes in bottom center area
	bottomEntities := []TextEntity{}
	for _, entity := range entities {
		if entity.Y < 100 { // Y < 100 (bottom area)
			bottomEntities = append(bottomEntities, entity)
		}
	}

	if len(bottomEntities) == 0 {
		// Use bottom half if no entities found below Y=100
		for i := 0; i < len(entities)-1; i++ {
			for j := i + 1; j < len(entities); j++ {
				if entities[i].Y > entities[j].Y {
					entities[i], entities[j] = entities[j], entities[i]
				}
			}
		}
		midPoint := len(entities) / 2
		bottomEntities = entities[:midPoint]
	}

	// Look for candidates in center area (avoid far right where revision notes might be)
	type centerCandidate struct {
		value string
		x     float64
		y     float64
	}
	centerCandidates := []centerCandidate{}

	for _, entity := range bottomEntities {
		if entity.X < 500 { // Avoid far right area where revision notes typically are
			match := pipeClassPattern.FindString(strings.TrimSpace(entity.Content))
			if match != "" {
				centerCandidates = append(centerCandidates, centerCandidate{match, entity.X, entity.Y})
			}
		}
	}

	if len(centerCandidates) > 0 {
		// Prefer candidates in the center-left area (where DESIGN DATA typically is)
		for i := 0; i < len(centerCandidates)-1; i++ {
			for j := i + 1; j < len(centerCandidates); j++ {
				if centerCandidates[i].x > centerCandidates[j].x {
					centerCandidates[i], centerCandidates[j] = centerCandidates[j], centerCandidates[i]
				}
			}
		}
		return centerCandidates[0].value
	}

	return ""
}

// extractWeldsFromRawContent parses polylines and detects weld symbols
func extractWeldsFromRawContent(rawContent []byte) (int, error) {
	segments, err := parsePolylineSegmentsOptimized(string(rawContent))
	if err != nil {
		return 0, err
	}
	
	weldSymbols := detectWeldSymbols(segments)
	return len(weldSymbols), nil
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

// parsePolylineSegmentsOptimized extracts polyline segments from DXF content
func parsePolylineSegmentsOptimized(content string) ([]PolylineSegment, error) {
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

// detectWeldSymbols finds pairs of crossed polyline segments with matching lengths
func detectWeldSymbols(segments []PolylineSegment) []WeldSymbol {
	var weldSymbols []WeldSymbol
	
	if len(segments) == 0 {
		return weldSymbols
	}
	
	// Check all pairs of segments (already filtered to target lengths)
	for i := 0; i < len(segments); i++ {
		for j := i + 1; j < len(segments); j++ {
			seg1 := segments[i]
			seg2 := segments[j]
			
			// Check if lengths match known weld symbol pairs
			if !lengthsMatch(seg1.Length, seg2.Length) {
				continue
			}
			
			// Check if segments intersect (crossed)
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
	
	// Remove duplicates (same location)
	return removeDuplicateSymbols(weldSymbols)
}

// removeDuplicateSymbols removes weld symbols that are too close to each other
func removeDuplicateSymbols(symbols []WeldSymbol) []WeldSymbol {
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

// writeWeldCSVs generates weld detection CSV files
func writeWeldCSVs(results []WeldResult, outputDir string) error {
	// Write weld counts CSV
	weldCountsFile := filepath.Join(outputDir, "0005_WELD_COUNTS.csv")
	if err := writeWeldCountsCSV(weldCountsFile, results); err != nil {
		return fmt.Errorf("error writing weld counts CSV: %v", err)
	}
	
	fmt.Printf("Wrote WELD COUNTS data to: %s (%d files)\n", weldCountsFile, len(results))
	return nil
}

// writeWeldCountsCSV writes the weld counts CSV file
func writeWeldCountsCSV(filename string, results []WeldResult) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header
	header := []string{
		"FilePath", "FileName", "DrawingNo", "PipeClass", 
		"WeldCount", "ProcessingTime", "Error",
	}
	if err := writer.Write(header); err != nil {
		return err
	}

	// Write data
	for _, result := range results {
		record := []string{
			result.FilePath,
			result.FileName,
			result.DrawingNo,
			result.PipeClass,
			strconv.Itoa(result.WeldCount),
			fmt.Sprintf("%.3f", result.ProcessingTime),
			result.Error,
		}
		if err := writer.Write(record); err != nil {
			return err
		}
	}

	return nil
}
