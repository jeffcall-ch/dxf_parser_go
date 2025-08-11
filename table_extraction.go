package main

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// isNumber checks if a string represents a numeric value
func isNumber(text string) bool {
	_, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
	return err == nil
}

// TableRow represents a row of data with Y coordinate for sorting
type TableRow struct {
	Y    float64
	Data []TableCell
}

// TableCell represents a cell with its X position and content
type TableCell struct {
	X    float64
	Text string
}

func extractTable(textEntities []TextEntity, tableTitle string) ([]string, [][]string) {
	const maxCols = 20
	const maxRows = 100

	// Find title
	var titleEntity *TextEntity
	var startX, titleY *float64

	for i := range textEntities {
		entity := &textEntities[i]
		if strings.Contains(strings.ToLower(entity.Content), strings.ToLower(tableTitle)) {
			startX = &entity.X
			titleY = &entity.Y
			titleEntity = entity
			debugPrint(fmt.Sprintf("[DEBUG] Table title '%s' found at X=%f, Y=%f, text='%s'", tableTitle, entity.X, entity.Y, entity.Content))
			break
		}
	}

	if titleEntity == nil || startX == nil {
		debugPrint(fmt.Sprintf("[DEBUG] Table title '%s' not found.", tableTitle))
		return []string{}, [][]string{}
	}

	// Filter entities based on table type and position
	filteredEntities := []TextEntity{}
	minX := *startX

	if strings.ToLower(tableTitle) == "cut pipe length" {
		// Allow data to the left of the title for cut pipe length table
		minX = *startX - 50
	}

	for _, entity := range textEntities {
		if entity.Y >= *titleY { // Skip rows at or above title
			continue
		}
		if entity.X >= minX {
			filteredEntities = append(filteredEntities, entity)
		}
	}

	// Group entities by Y coordinate (rows)
	rowsDict := make(map[float64][]TableCell)
	for _, entity := range filteredEntities {
		yKey := math.Round(entity.Y*10) / 10 // Round to 1 decimal place
		if _, exists := rowsDict[yKey]; !exists {
			rowsDict[yKey] = []TableCell{}
		}
		rowsDict[yKey] = append(rowsDict[yKey], TableCell{X: entity.X, Text: entity.Content})
	}

	// Sort rows by Y coordinate (descending - top to bottom)
	type rowData struct {
		y     float64
		cells []TableCell
	}
	
	sortedRows := []rowData{}
	for y, cells := range rowsDict {
		sortedRows = append(sortedRows, rowData{y: y, cells: cells})
	}
	
	sort.Slice(sortedRows, func(i, j int) bool {
		return sortedRows[i].y > sortedRows[j].y // Descending Y
	})

	// For each row, sort cells by X coordinate (left to right)
	tableRows := [][]string{}
	for idx, row := range sortedRows {
		// Sort cells by X coordinate
		sort.Slice(row.cells, func(i, j int) bool {
			return row.cells[i].X < row.cells[j].X
		})

		// Extract text content
		rowTexts := make([]string, len(row.cells))
		for i, cell := range row.cells {
			rowTexts[i] = cell.Text
		}

		// Debug output for specific cases
		if strings.ToLower(tableTitle) == "cut pipe length" && idx == 2 {
			xs := make([]float64, len(row.cells))
			for i, cell := range row.cells {
				xs[i] = cell.X
			}
			debugPrint(fmt.Sprintf("[DEBUG] Extracted row %d at y=%f, x=%v: %v <-- 3RD ROW BELOW 'CUT PIPE LENGTH'", idx+1, row.y, xs, rowTexts))
		} else if idx < 3 { // Only show first 3 rows for debugging
			xs := make([]float64, len(row.cells))
			for i, cell := range row.cells {
				xs[i] = cell.X
			}
			debugPrint(fmt.Sprintf("[DEBUG] Extracted row %d at y=%f, x=%v: %v", idx+1, row.y, xs, rowTexts))
		}

		tableRows = append(tableRows, rowTexts)
	}

	if strings.ToLower(tableTitle) == "cut pipe length" {
		debugPrint(fmt.Sprintf("[DEBUG] Total rows extracted for 'CUT PIPE LENGTH': %d", len(tableRows)))
	}

	// Process headers - merge first two rows
	var header []string
	var dataRows [][]string

	if len(tableRows) >= 2 {
		// Merge first two rows as header
		maxHeaderCols := len(tableRows[0])
		if len(tableRows[1]) > maxHeaderCols {
			maxHeaderCols = len(tableRows[1])
		}

		header = make([]string, maxHeaderCols)
		for i := 0; i < maxHeaderCols; i++ {
			h1 := ""
			h2 := ""
			if i < len(tableRows[0]) {
				h1 = tableRows[0][i]
			}
			if i < len(tableRows[1]) {
				h2 = tableRows[1][i]
			}

			// Special handling for CUT PIPE LENGTH table headers
			if strings.ToLower(tableTitle) == "cut pipe length" {
				merged := mergeHeaderForCutPipeLength(h1, h2)
				header[i] = merged
			} else {
				merged := strings.TrimSpace(h1 + " " + h2)
				header[i] = merged
			}
		}
		dataRows = tableRows[2:]
	} else {
		if len(tableRows) > 0 {
			header = tableRows[0]
		}
		if len(tableRows) > 1 {
			dataRows = tableRows[1:]
		}
	}

	// Process based on table type
	if strings.Contains(strings.ToUpper(tableTitle), "ERECTION MATERIALS") {
		dataRows = processErectionMaterialsTable(dataRows)
		// Update header to include the new CATEGORY column
		if len(header) > 0 {
			// Insert CATEGORY at position 5 (column F)
			newHeader := make([]string, len(header)+1)
			copy(newHeader[:5], header[:5])
			newHeader[5] = "CATEGORY"
			if len(header) > 5 {
				copy(newHeader[6:], header[5:])
			}
			header = newHeader
		}
	}

	// For CUT PIPE LENGTH, filter rows with '<' and apply validation
	if strings.ToLower(tableTitle) == "cut pipe length" {
		keptRows := [][]string{}
		for _, row := range dataRows {
			rowStr := strings.Join(row, "")
			if strings.Contains(rowStr, "<") {
				keptRows = append(keptRows, row)
			}
		}
		debugPrint(fmt.Sprintf("[DEBUG] Kept rows for 'CUT PIPE LENGTH':"))
		for i, r := range keptRows {
			if i < 2 { // Only show first 2 rows for performance
				debugPrint(fmt.Sprintf("[DEBUG] %v", r))
			}
		}
		dataRows = keptRows

		// Apply column validation and correction for CUT PIPE LENGTH
		correctedRows := [][]string{}
		for _, row := range dataRows {
			correctedRow := validateAndCorrectCutLengthRow(row)
			correctedRows = append(correctedRows, correctedRow)
		}
		dataRows = correctedRows
	}

	// For CUT PIPE LENGTH, stop at first empty row
	if strings.Contains(strings.ToUpper(tableTitle), "CUT PIPE LENGTH") {
		newDataRows := [][]string{}
		for _, row := range dataRows {
			allEmpty := true
			for _, cell := range row {
				if strings.TrimSpace(cell) != "" {
					allEmpty = false
					break
				}
			}
			if allEmpty {
				break
			}
			newDataRows = append(newDataRows, row)
		}
		dataRows = newDataRows
	}

	// Pad all rows to header length
	paddedRows := [][]string{}
	for _, row := range dataRows {
		paddedRow := make([]string, len(header))
		for i := 0; i < len(header); i++ {
			if i < len(row) {
				paddedRow[i] = row[i]
			} else {
				paddedRow[i] = ""
			}
		}
		paddedRows = append(paddedRows, paddedRow)
	}

	return header, paddedRows
}

func mergeHeaderForCutPipeLength(h1, h2 string) string {
	if h1 != "" && h2 != "" {
		if h1 == "N.S." && h2 == "(MM)" {
			return "N.S. (MM)"
		} else if h1 == "PIECE" && h2 == "NO" {
			return "PIECE NO"
		} else if h1 == "CUT" && h2 == "LENGTH" {
			return "CUT LENGTH"
		} else if h1 == "REMARKS" && h2 == "NO" {
			return "REMARKS" // Don't add 'NO' to REMARKS
		} else if h1 == "REMARKS" && h2 == "" {
			return "REMARKS"
		} else if h1 == "PIECE" && h2 == "LENGTH" { // Should be 'PIECE NO'
			return "PIECE NO"
		} else if h1 == "CUT" && h2 == "(MM)" { // Should be 'CUT LENGTH'
			return "CUT LENGTH"
		} else {
			return strings.TrimSpace(h1 + " " + h2)
		}
	} else if h1 != "" && h2 == "" {
		// Handle single header values for the right side columns
		if h1 == "PIECE" {
			return "PIECE NO"
		} else if h1 == "CUT" {
			return "CUT LENGTH"
		} else if h1 == "N.S." {
			return "N.S. (MM)"
		} else {
			return h1
		}
	} else if h2 != "" && h1 == "" {
		return h2
	}
	return ""
}

func processErectionMaterialsTable(dataRows [][]string) [][]string {
	// For ERECTION MATERIALS, stop at 'TOTAL WEIGHT' row
	endIdx := len(dataRows)
	for i, row := range dataRows {
		for _, cell := range row {
			if strings.Contains(strings.ToUpper(cell), "TOTAL WEIGHT") {
				endIdx = i + 1
				break
			}
		}
		if endIdx != len(dataRows) {
			break
		}
	}
	dataRows = dataRows[:endIdx]

	// Process ERECTION MATERIALS to move categories from column A to column F
	processedRows := [][]string{}
	currentCategory := ""

	for _, row := range dataRows {
		if len(row) == 0 {
			continue
		}

		// Check if this row is a category header or total row
		// (has content in first column but empty in other key columns)
		isCategory := false
		isTotalRow := false
		if row[0] != "" {
			// Check if this is a total row
			if row[0] == "TOTAL ERECTION WEIGHT" || row[0] == "TOTAL WEIGHT" {
				isTotalRow = true
				isCategory = true // Treat total rows as special category rows
			} else {
				// Check for regular category rows
				isEmpty := true
				if len(row) > 1 && row[1] != "" {
					isEmpty = false
				}
				if len(row) > 3 && row[3] != "" {
					isEmpty = false
				}
				if isEmpty {
					isCategory = true
				}
			}
		}

		if isCategory {
			// This is likely a category header like "PIPE", "FITTINGS", etc.
			if isTotalRow {
				// For total rows, move the total type to column F and weight value to column E
				newRow := make([]string, 6) // Create exactly 6 columns (A-F)
				
				totalType := row[0] // Save the total type
				weightValue := ""
				if len(row) > 1 {
					weightValue = row[1] // Save the weight value from column B
				}

				// Leave columns A-D empty, put weight in E (index 4), total type in F (index 5)
				newRow[0] = "" // Column A
				newRow[1] = "" // Column B  
				newRow[2] = "" // Column C
				newRow[3] = "" // Column D
				newRow[4] = weightValue // Column E (WEIGHT)
				newRow[5] = totalType   // Column F (CATEGORY)
				
				processedRows = append(processedRows, newRow)
			} else {
				// Regular category header
				if row[0] != "TOTAL ERECTION WEIGHT" && row[0] != "TOTAL WEIGHT" {
					currentCategory = row[0]
					debugPrint(fmt.Sprintf("[DEBUG] Found category: '%s'", currentCategory))
					continue // Skip category header rows, don't add to processed_rows
				}
			}
		} else {
			// This is a regular data row, add the current category to column F
			newRow := make([]string, len(row))
			copy(newRow, row)

			// Detect and fix missing N.S. column issue (conservative approach)
			// Expected structure: PT_NO | DESCRIPTION | N.S. | QTY | WEIGHT | CATEGORY
			//                    [0]   | [1]         | [2]  | [3] | [4]    | [5]
			
			// Only attempt correction if we have exactly 5 columns (missing one column)
			// and the row appears to be a component row (not a category header)
			if len(newRow) == 5 && newRow[0] != "" && newRow[1] != "" {
				
				// Working backwards from the structure:
				// newRow[4] should be WEIGHT (always present - number or "---")
				// newRow[3] should be QTY (always present)
				// newRow[2] should be N.S. (sometimes missing)
				
				col2 := strings.TrimSpace(newRow[2]) // What should be N.S.
				col4 := strings.TrimSpace(newRow[4]) // What should be WEIGHT
				
				// Check if col4 looks like a valid WEIGHT value
				isCol4ValidWeight := false
				if col4 == "---" || col4 == "" {
					isCol4ValidWeight = true
				} else if val, err := strconv.ParseFloat(col4, 64); err == nil && val >= 0 {
					isCol4ValidWeight = true
				}
				
				// If col4 is valid weight, check if col2 looks like it contains QTY data
				// instead of N.S. data (indicating missing N.S. column)
				if isCol4ValidWeight {
					// N.S. values are typically: empty, numbers like "25", or "number x number" format like "25 x 15"
					// QTY values are typically: decimal numbers, numbers with "M" suffix, small integers
					
					isCol2LikelyQty := false
					
					// Check if col2 looks like QTY (pipe length with M suffix)
					if strings.HasSuffix(col2, "M") {
						isCol2LikelyQty = true
					}
					
					// Check if col2 is a small integer that's likely QTY, not N.S.
					// N.S. (nominal size) is typically 15, 25, 50, etc. (pipe sizes)
					// QTY can be small numbers like 1, 2, 3, etc.
					if val, err := strconv.ParseFloat(col2, 64); err == nil {
						// If it's a small number (< 10) and doesn't look like a standard pipe size
						if val > 0 && val < 10 && val != 15 && val != 25 && val != 50 && val != 80 && val != 100 {
							// This is likely a QTY, not N.S.
							isCol2LikelyQty = true
						}
					}
					
					// Check if col2 contains decimal values which are more likely QTY than N.S.
					if strings.Contains(col2, ".") {
						if val, err := strconv.ParseFloat(col2, 64); err == nil && val > 0 {
							isCol2LikelyQty = true
						}
					}
					
					// Only apply fix if we're confident this is a missing N.S. case
					if isCol2LikelyQty {
						debugPrint(fmt.Sprintf("[DEBUG] Detected missing N.S. column in row: %v (Category: %s)", newRow, currentCategory))
						
						// Shift data right to insert missing N.S. column
						correctedRow := make([]string, 6)
						correctedRow[0] = newRow[0] // PT_NO
						correctedRow[1] = newRow[1] // DESCRIPTION  
						correctedRow[2] = ""        // N.S. (missing, leave empty)
						correctedRow[3] = newRow[2] // QTY (was in N.S. position)
						correctedRow[4] = newRow[3] // WEIGHT (was in QTY position) 
						correctedRow[5] = newRow[4] // Keep any additional data
						
						newRow = correctedRow
						debugPrint(fmt.Sprintf("[DEBUG] Corrected row: %v", newRow))
					}
				}
			}

			// Remove "M" from pipe length cells (QTY column - now correctly in column D)
			if len(newRow) > 3 && newRow[3] != "" {
				// Remove "M" suffix from quantity values like "2.4M" -> "2.4"
				newRow[3] = strings.TrimSuffix(newRow[3], "M")
			}

			// Ensure we have enough columns (at least 6 for A-F)
			for len(newRow) < 6 {
				newRow = append(newRow, "")
			}

			// Put category in column F (index 5)
			newRow[5] = currentCategory
			
			processedRows = append(processedRows, newRow)
		}
	}

	return processedRows
}


