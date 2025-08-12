package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Global debug flag
var debugMode = false

// Compiled regex patterns for performance
var (
	pieceNumberPattern = regexp.MustCompile(`^<\d+>$`)
	numberPattern      = regexp.MustCompile(`^\d+(\.\d+)?$`)
	kksPattern         = regexp.MustCompile(`\b\d[A-Z]{3}\d{2}BR\d{3}\b`)
	pipeClassPattern   = regexp.MustCompile(`\b[A-Z]{4}\b`)
)

// DXFResult represents the extracted data from a single DXF file
type DXFResult struct {
	DrawingNo      string      `json:"drawing_no"`
	PipeClass      string      `json:"pipe_class"`
	MatHeader      []string    `json:"mat_header"`
	MatRows        [][]string  `json:"mat_rows"`
	CutHeader      []string    `json:"cut_header"`
	CutRows        [][]string  `json:"cut_rows"`
	Error          string      `json:"error"`
	ProcessingTime float64     `json:"processing_time"`
	Filename       string      `json:"filename"`
	FilePath       string      `json:"file_path"`
}

// SummaryRow for the summary CSV output
type SummaryRow struct {
	FilePath       string  `json:"file_path"`
	Filename       string  `json:"filename"`
	DrawingNo      string  `json:"drawing_no"`
	PipeClass      string  `json:"pipe_class"`
	MatRows        int     `json:"mat_rows"`
	CutRows        int     `json:"cut_rows"`
	MatMissing     bool    `json:"mat_missing"`
	CutMissing     bool    `json:"cut_missing"`
	Error          string  `json:"error"`
	ProcessingTime float64 `json:"processing_time"`
}

func debugPrint(message string) {
	if debugMode {
		fmt.Println(message)
	}
}

// Write all output files
func writeOutputFiles(directory string, materialRows, cutRows [][]string, summary []SummaryRow, matHeader, cutHeader []string) error {
	
	// Write ERECTION MATERIALS CSV
	if len(materialRows) > 0 {
		matFilename := filepath.Join(directory, "0001_ERECTION_MATERIALS.csv")
		if err := writeCSV(matFilename, matHeader, materialRows); err != nil {
			return fmt.Errorf("error writing materials CSV: %v", err)
		}
		fmt.Printf("Wrote ERECTION MATERIALS data to: %s (%d rows)\n", matFilename, len(materialRows))
		
		// Post-process to fix missing N.S. columns
		if err := fixMissingNSColumns(matFilename); err != nil {
			return fmt.Errorf("error fixing missing N.S. columns: %v", err)
		}
	}

	// Write CUT PIPE LENGTH CSV
	if len(cutRows) > 0 {
		cutFilename := filepath.Join(directory, "0002_CUT_PIPE_LENGTH.csv")
		if err := writeCSV(cutFilename, cutHeader, cutRows); err != nil {
			return fmt.Errorf("error writing cut pipe CSV: %v", err)
		}
		fmt.Printf("Wrote CUT PIPE LENGTH data to: %s (%d rows)\n", cutFilename, len(cutRows))
	}

	// Write AGGREGATED MATERIALS CSV
	if len(materialRows) > 0 {
		aggHeader, aggRows := createAggregatedMaterials(materialRows, matHeader)
		aggFilename := filepath.Join(directory, "0003_AGGREGATED_MATERIALS.csv")
		if err := writeCSV(aggFilename, aggHeader, aggRows); err != nil {
			return fmt.Errorf("error writing aggregated materials CSV: %v", err)
		}
		fmt.Printf("Wrote AGGREGATED MATERIALS data to: %s (%d rows)\n", aggFilename, len(aggRows))
	}

	// Write summary CSV
	summaryFilename := filepath.Join(directory, "0004_SUMMARY.csv")
	if err := writeSummaryCSV(summaryFilename, summary); err != nil {
		return fmt.Errorf("error writing summary CSV: %v", err)
	}
	fmt.Printf("Wrote processing summary to: %s (%d files)\n", summaryFilename, len(summary))

	return nil
}

// Write a generic CSV file
func writeCSV(filename string, header []string, rows [][]string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header
	if err := writer.Write(header); err != nil {
		return err
	}

	// Write rows
	for _, row := range rows {
		if err := writer.Write(row); err != nil {
			return err
		}
	}

	return nil
}

// Write summary CSV
func writeSummaryCSV(filename string, summary []SummaryRow) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header
	header := []string{
		"FilePath", "Filename", "DrawingNo", "PipeClass", 
		"MatRows", "CutRows", "MatMissing", "CutMissing", 
		"Error", "ProcessingTime",
	}
	if err := writer.Write(header); err != nil {
		return err
	}

	// Write rows
	for _, row := range summary {
		csvRow := []string{
			row.FilePath,
			row.Filename,
			row.DrawingNo,
			row.PipeClass,
			strconv.Itoa(row.MatRows),
			strconv.Itoa(row.CutRows),
			strconv.FormatBool(row.MatMissing),
			strconv.FormatBool(row.CutMissing),
			row.Error,
			fmt.Sprintf("%.3f", row.ProcessingTime),
		}
		if err := writer.Write(csvRow); err != nil {
			return err
		}
	}

	return nil
}

// Process files sequentially
func processFilesSequential(files []string, debug bool) []DXFResult {
	results := make([]DXFResult, 0, len(files))
	
	for i, filePath := range files {
		if debug {
			fmt.Printf("[%d/%d] Processing: %s\n", i+1, len(files), filepath.Base(filePath))
		} else {
			fmt.Printf("Processing file %d/%d: %s\n", i+1, len(files), filepath.Base(filePath))
		}
		
		result := processDXFFile(filePath)
		results = append(results, result)
	}
	
	return results
}

// Process files in parallel
func processFilesParallel(files []string, workers int, debug bool) []DXFResult {
	jobs := make(chan string, len(files))
	results := make(chan DXFResult, len(files))
	
	// Start workers
	for w := 0; w < workers; w++ {
		go func() {
			for filePath := range jobs {
				result := processDXFFile(filePath)
				results <- result
			}
		}()
	}
	
	// Send jobs
	for _, filePath := range files {
		jobs <- filePath
	}
	close(jobs)
	
	// Collect results
	var allResults []DXFResult
	for i := 0; i < len(files); i++ {
		result := <-results
		allResults = append(allResults, result)
		
		if debug {
			fmt.Printf("[%d/%d] Completed: %s\n", i+1, len(files), filepath.Base(result.FilePath))
		} else {
			fmt.Printf("Completed file %d/%d: %s\n", i+1, len(files), filepath.Base(result.FilePath))
		}
	}
	
	return allResults
}

// Print final summary
func printFinalSummary(totalFiles, successfulFiles int, totalTime, totalProcessingTime float64, workers, matRows, cutRows int, directory string) {
	
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("PROCESSING COMPLETE")
	fmt.Println(strings.Repeat("=", 60))
	
	fmt.Printf("Directory: %s\n", directory)
	fmt.Printf("Total Files: %d\n", totalFiles)
	fmt.Printf("Successful: %d\n", successfulFiles)
	fmt.Printf("Failed: %d\n", totalFiles-successfulFiles)
	fmt.Printf("Workers: %d\n", workers)
	fmt.Printf("Total Material Rows: %d\n", matRows)
	fmt.Printf("Total Cut Pipe Rows: %d\n", cutRows)
	fmt.Printf("Wall Clock Time: %.3f seconds\n", totalTime)
	fmt.Printf("Total Processing Time: %.3f seconds\n", totalProcessingTime)
	
	if workers > 1 && totalProcessingTime > 0 {
		efficiency := (totalProcessingTime / totalTime) * 100 / float64(workers)
		fmt.Printf("Parallel Efficiency: %.1f%%\n", efficiency)
	}
	
	if successfulFiles > 0 {
		avgTime := totalProcessingTime / float64(successfulFiles)
		fmt.Printf("Average Time per File: %.3f seconds\n", avgTime)
	}
	
	fmt.Println(strings.Repeat("=", 60))
}

// AggregatedItem represents an aggregated material item
type AggregatedItem struct {
	Description string
	NS          string
	TotalQty    float64
	Weight      string
	Category    string
}

// createAggregatedMaterials combines materials by description and organizes by category
func createAggregatedMaterials(materialRows [][]string, matHeader []string) ([]string, [][]string) {
	// Map to store aggregated items by description
	itemMap := make(map[string]*AggregatedItem)
	
	// Process each material row (skip total rows)
	for _, row := range materialRows {
		if len(row) < 6 {
			continue
		}
		
		// Skip total rows
		if len(row) >= 5 && (strings.Contains(row[4], "TOTAL") || row[1] == "") {
			continue
		}
		
		description := row[1]  // Column B - Component Description
		ns := row[2]          // Column C - N.S.
		qtyStr := row[3]      // Column D - QTY
		weight := row[4]      // Column E - WEIGHT
		category := row[5]    // Column F - CATEGORY
		
		if description == "" || category == "" {
			continue
		}
		
		// Parse quantity (handle various formats)
		qty := parseQuantity(qtyStr)
		
		// Create unique key based on description and N.S.
		key := description + "|" + ns
		
		if item, exists := itemMap[key]; exists {
			// Add to existing item
			item.TotalQty += qty
		} else {
			// Create new item
			itemMap[key] = &AggregatedItem{
				Description: description,
				NS:          ns,
				TotalQty:    qty,
				Weight:      weight, // Use weight from first occurrence
				Category:    category,
			}
		}
	}
	
	// Convert map to slice and organize by category
	var items []*AggregatedItem
	for _, item := range itemMap {
		items = append(items, item)
	}
	
	// Sort by category priority and then by description
	sortItemsByCategory(items)
	
	// Create header and rows
	header := []string{"DESCRIPTION", "N.S.", "TOTAL QTY", "UNIT WEIGHT", "CATEGORY"}
	var rows [][]string
	
	for _, item := range items {
		qtyStr := formatQuantity(item.TotalQty)
		row := []string{
			item.Description,
			item.NS,
			qtyStr,
			item.Weight,
			item.Category,
		}
		rows = append(rows, row)
	}
	
	return header, rows
}

// parseQuantity parses quantity string and returns float64
func parseQuantity(qtyStr string) float64 {
	if qtyStr == "" || qtyStr == "---" {
		return 0
	}
	
	// Remove units and spaces
	cleaned := strings.TrimSpace(qtyStr)
	cleaned = strings.ReplaceAll(cleaned, "M", "")
	cleaned = strings.ReplaceAll(cleaned, "m", "")
	
	// Try to parse as float
	if qty, err := strconv.ParseFloat(cleaned, 64); err == nil {
		return qty
	}
	
	return 0
}

// formatQuantity formats quantity for display
func formatQuantity(qty float64) string {
	if qty == float64(int64(qty)) {
		return strconv.Itoa(int(qty))
	}
	return strconv.FormatFloat(qty, 'f', 1, 64)
}

// sortItemsByCategory sorts items by category priority and description
func sortItemsByCategory(items []*AggregatedItem) {
	categoryOrder := map[string]int{
		"PIPE":                        1,
		"FITTINGS":                   2,
		"VALVES / IN-LINE ITEMS":     3,
		"SUPPORTS":                   4,
		"MISCELLANEOUS COMPONENTS":   5,
	}
	
	sort.Slice(items, func(i, j int) bool {
		// First sort by category
		orderI := categoryOrder[items[i].Category]
		orderJ := categoryOrder[items[j].Category]
		if orderI == 0 {
			orderI = 999 // Unknown categories go to end
		}
		if orderJ == 0 {
			orderJ = 999
		}
		
		if orderI != orderJ {
			return orderI < orderJ
		}
		
		// Then sort by description
		return items[i].Description < items[j].Description
	})
}

// fixMissingNSColumns post-processes the ERECTION MATERIALS CSV to fix missing N.S. columns
// and clean QTY values by removing "M" suffixes.
// It looks for rows where PT NO has a value but WEIGHT is empty, indicating missing N.S. column
func fixMissingNSColumns(filename string) error {
	// Read the CSV file
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.LazyQuotes = true
	reader.TrimLeadingSpace = true

	records, err := reader.ReadAll()
	if err != nil {
		return err
	}

	if len(records) == 0 {
		return nil // Nothing to process
	}

	header := records[0]
	rows := records[1:]
	
	// Find column indices
	ptNoIdx := -1
	nsIdx := -1
	qtyIdx := -1
	weightIdx := -1
	
	for i, col := range header {
		switch strings.TrimSpace(col) {
		case "PT NO":
			ptNoIdx = i
		case "N.S.":
			nsIdx = i
		case "QTY":
			qtyIdx = i
		case "WEIGHT":
			weightIdx = i
		}
	}
	
	if ptNoIdx == -1 || nsIdx == -1 || qtyIdx == -1 || weightIdx == -1 {
		debugPrint("[DEBUG] Could not find required columns for N.S. correction")
		return nil // Can't process without proper column structure
	}
	
	correctedRows := [][]string{}
	correctionCount := 0
	
	for _, row := range rows {
		// Ensure row has enough columns
		for len(row) <= weightIdx {
			row = append(row, "")
		}
		
		// Check if this is a component row (PT NO has value) AND WEIGHT is empty
		ptNo := strings.TrimSpace(row[ptNoIdx])
		weight := strings.TrimSpace(row[weightIdx])
		
		if ptNo != "" && weight == "" {
			// This row has missing N.S. column - shift columns right
			debugPrint(fmt.Sprintf("[DEBUG] Fixing missing N.S. column for PT NO '%s'", ptNo))
			
			newRow := make([]string, len(row))
			copy(newRow, row)
			
			// Shift: N.S. → QTY, QTY → WEIGHT, leave N.S. empty
			newRow[weightIdx] = row[qtyIdx] // Move QTY to WEIGHT
			newRow[qtyIdx] = row[nsIdx]     // Move N.S. to QTY
			newRow[nsIdx] = ""              // Clear N.S. (it was missing)
			
			correctedRows = append(correctedRows, newRow)
			correctionCount++
		} else {
			// No correction needed
			correctedRows = append(correctedRows, row)
		}
	}
	
	// Clean QTY column - remove "M" suffixes and ensure numeric values
	qtyCleanCount := 0
	for i, row := range correctedRows {
		if len(row) > qtyIdx {
			qty := strings.TrimSpace(row[qtyIdx])
			if qty != "" {
				// Remove "M" suffix if present
				if strings.HasSuffix(strings.ToUpper(qty), "M") {
					cleanQty := strings.TrimSuffix(strings.ToUpper(qty), "M")
					cleanQty = strings.TrimSpace(cleanQty)
					if cleanQty != qty {
						debugPrint(fmt.Sprintf("[DEBUG] Cleaning QTY: '%s' → '%s'", qty, cleanQty))
						correctedRows[i][qtyIdx] = cleanQty
						qtyCleanCount++
					}
				}
			}
		}
	}
	
	if correctionCount > 0 || qtyCleanCount > 0 {
		if correctionCount > 0 {
			debugPrint(fmt.Sprintf("[DEBUG] Fixed %d rows with missing N.S. columns", correctionCount))
		}
		if qtyCleanCount > 0 {
			debugPrint(fmt.Sprintf("[DEBUG] Cleaned %d QTY values (removed 'M' suffixes)", qtyCleanCount))
		}
		
		// Write back the corrected CSV
		allRecords := [][]string{header}
		allRecords = append(allRecords, correctedRows...)
		
		return writeCSV(filename, header, correctedRows)
	}
	
	return nil // No corrections needed
}

// Process a single DXF file with optional caching for weld detection
func processDXFFileWithCaching(filepath string, weldFlag bool) (DXFResult, *FileCache) {
	start := time.Now()
	result := DXFResult{
		Filename: filepath,
		FilePath: filepath,
	}
	
	var cache *FileCache
	if weldFlag {
		cache = &FileCache{}
		// Read raw content for weld detection
		if rawContent, err := os.ReadFile(filepath); err == nil {
			cache.RawContent = rawContent
		}
	}

	debugPrint(fmt.Sprintf("[DEBUG] Opening DXF file: %s", filepath))

	// Use our existing Go DXF parser
	parser := NewDXFParser(1) // Use single worker for individual file processing
	textEntities, err := parser.ParseFile(filepath)
	if err != nil {
		result.Error = fmt.Sprintf("Failed to parse DXF file: %v", err)
		result.ProcessingTime = time.Since(start).Seconds()
		return result, cache
	}
	
	// Cache text entities for weld detection if needed
	if weldFlag {
		cache.TextEntities = textEntities
		cache.DrawingNo = findDrawingNo(textEntities)
		cache.PipeClass = findPipeClass(textEntities)
	}

	drawingNo := findDrawingNo(textEntities)
	pipeClass := findPipeClass(textEntities)

	matHeader, matRows := extractTable(textEntities, "ERECTION MATERIALS")
	cutHeader, cutRows := extractTable(textEntities, "CUT PIPE LENGTH")

	// Add Drawing-No. and Pipe Class to each row
	if len(matRows) > 0 {
		result.MatHeader = append(matHeader, "Drawing-No.", "Pipe Class")
		result.MatRows = make([][]string, len(matRows))
		for i, row := range matRows {
			result.MatRows[i] = append(row, drawingNo, pipeClass)
		}
	}

	if len(cutRows) > 0 {
		// Extract pipe descriptions from material table for cut length table
		pipeDescriptions := extractPipeDescriptions(matRows)
		
		// Convert to single-row format with pipe descriptions
		result.CutHeader, result.CutRows = convertCutLengthToSingleRowFormat(cutHeader, cutRows, drawingNo, pipeClass, pipeDescriptions)
	}

	result.DrawingNo = drawingNo
	result.PipeClass = pipeClass
	result.ProcessingTime = time.Since(start).Seconds()

	debugPrint(fmt.Sprintf("[DEBUG] Extracted %d material rows and %d cut length rows from %s", len(result.MatRows), len(result.CutRows), filepath))
	debugPrint(fmt.Sprintf("[DEBUG] Drawing No: '%s', Pipe Class: '%s'", drawingNo, pipeClass))

	return result, cache
}

// Process files sequentially with optional caching for weld detection
func processFilesSequentialWithCaching(files []string, debug bool, weldFlag bool) ([]DXFResult, map[string]FileCache) {
	results := make([]DXFResult, 0, len(files))
	var fileCache map[string]FileCache
	
	if weldFlag {
		fileCache = make(map[string]FileCache)
	}
	
	for i, filePath := range files {
		if debug {
			fmt.Printf("[%d/%d] Processing: %s\n", i+1, len(files), filepath.Base(filePath))
		} else {
			fmt.Printf("Processing file %d/%d: %s\n", i+1, len(files), filepath.Base(filePath))
		}
		
		result, cache := processDXFFileWithCaching(filePath, weldFlag)
		results = append(results, result)
		
		if weldFlag && cache != nil {
			fileCache[filePath] = *cache
		}
	}
	
	return results, fileCache
}

// Process files in parallel with optional caching for weld detection
func processFilesParallelWithCaching(files []string, workers int, debug bool, weldFlag bool) ([]DXFResult, map[string]FileCache) {
	jobs := make(chan string, len(files))
	type resultWithCache struct {
		result DXFResult
		cache  *FileCache
	}
	results := make(chan resultWithCache, len(files))
	
	// Start workers
	for w := 0; w < workers; w++ {
		go func() {
			for filePath := range jobs {
				result, cache := processDXFFileWithCaching(filePath, weldFlag)
				results <- resultWithCache{result: result, cache: cache}
			}
		}()
	}
	
	// Send jobs
	for _, filePath := range files {
		jobs <- filePath
	}
	close(jobs)
	
	// Collect results
	var allResults []DXFResult
	var fileCache map[string]FileCache
	
	if weldFlag {
		fileCache = make(map[string]FileCache)
	}
	
	for i := 0; i < len(files); i++ {
		resultWithCache := <-results
		allResults = append(allResults, resultWithCache.result)
		
		if weldFlag && resultWithCache.cache != nil {
			fileCache[resultWithCache.result.FilePath] = *resultWithCache.cache
		}
		
		if debug {
			fmt.Printf("[%d/%d] Completed: %s\n", i+1, len(files), filepath.Base(resultWithCache.result.FilePath))
		} else {
			fmt.Printf("Completed file %d/%d: %s\n", i+1, len(files), filepath.Base(resultWithCache.result.FilePath))
		}
	}
	
	return allResults, fileCache
}


