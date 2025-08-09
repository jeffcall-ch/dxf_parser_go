package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Write all output files
func writeOutputFiles(directory string, materialRows, cutRows [][]string, summary []SummaryRow, matHeader, cutHeader []string) error {
	
	// Write ERECTION MATERIALS CSV
	if len(materialRows) > 0 {
		matFilename := filepath.Join(directory, "0001_ERECTION_MATERIALS.csv")
		if err := writeCSV(matFilename, matHeader, materialRows); err != nil {
			return fmt.Errorf("error writing materials CSV: %v", err)
		}
		fmt.Printf("Wrote ERECTION MATERIALS data to: %s (%d rows)\n", matFilename, len(materialRows))
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


