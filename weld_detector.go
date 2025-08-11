package main

import (
	"bufio"
	"encoding/csv"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Helper function for minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

var (
	kksPattern = regexp.MustCompile(`\b\d[A-Z]{3}\d{2}BR\d{3}\b`)
	pipeClassPattern = regexp.MustCompile(`\b[A-Z]{4}\b`)
	globalQuiet = false // Global flag for quiet mode
)

// TextEntity represents a text entity extracted from a DXF file
type TextEntity struct {
	Content    string  `json:"content"`
	X          float64 `json:"x"`
	Y          float64 `json:"y"`
	Height     float64 `json:"height,omitempty"`
	EntityType string  `json:"entity_type"`
	Layer      string  `json:"layer,omitempty"`
}

// abs returns the absolute value of a float64
func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// debugPrint prints debug messages (simplified version for weld detector)
func debugPrint(msg string) {
	// Debug output disabled for production
	// fmt.Println(msg)
}

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
	PipeClass      string `json:"pipe_class"`
	PipeDescription string `json:"pipe_description"`
	PipeNS         string `json:"pipe_ns"`
	PipeQty        string `json:"pipe_qty"`
	MultiplePipesNote string `json:"multiple_pipes_note"`
	Error          string `json:"error,omitempty"`
	ProcessingTime float64 `json:"processing_time"`
}

// PipeInfo represents pipe information extracted from BOM
type PipeInfo struct {
	Description   string
	NS           string
	Qty          string
	PipeClass    string
	Count        int // How many pipe entries were found
	AllPipes     []string // All pipe descriptions for multiple pipes note
}

// extractPipeInfoFromBOM extracts pipe information from the ERECTION MATERIALS table
func extractPipeInfoFromBOM(textEntities []TextEntity) PipeInfo {
	// Extract ERECTION MATERIALS table
	header, rows := extractTable(textEntities, "ERECTION MATERIALS")
	
	pipeInfo := PipeInfo{
		Description: "No PIPE found",
		NS:         "",
		Qty:        "",
		PipeClass:  "",
		Count:      0,
		AllPipes:   []string{},
	}
	
	if len(rows) == 0 {
		return pipeInfo
	}
	
	// Find pipe class from drawing
	pipeInfo.PipeClass = findPipeClass(textEntities)
	
	// Find component description column index (should be "COMPONENT DESCRIPTION (MM)")
	descIndex := -1
	nsIndex := -1
	qtyIndex := -1
	categoryIndex := -1
	
	for i, col := range header {
		colUpper := strings.ToUpper(strings.TrimSpace(col))
		debugPrint(fmt.Sprintf("[DEBUG] Header[%d]: '%s' -> '%s'", i, col, colUpper))
		if strings.Contains(colUpper, "COMPONENT DESCRIPTION") {
			descIndex = i
		} else if strings.Contains(colUpper, "N.S.") {
			nsIndex = i
		} else if strings.Contains(colUpper, "QTY") {
			qtyIndex = i
		} else if strings.Contains(colUpper, "CATEGORY") {
			categoryIndex = i
		}
	}
	
	debugPrint(fmt.Sprintf("[DEBUG] Column indices: desc=%d, ns=%d, qty=%d, category=%d", descIndex, nsIndex, qtyIndex, categoryIndex))
	
	if descIndex == -1 {
		return pipeInfo
	}
	
	// Find all pipe entries
	pipeEntries := []struct {
		description string
		ns         string
		qty        string
	}{}
	
	for _, row := range rows {
		if len(row) <= descIndex {
			continue
		}
		
		// Check if this is a pipe component by looking at the CATEGORY column
		isPipe := false
		if categoryIndex >= 0 && len(row) > categoryIndex {
			category := strings.TrimSpace(row[categoryIndex])
			if strings.ToUpper(category) == "PIPE" {
				isPipe = true
			}
		}
		
		// If no CATEGORY column, fall back to checking description for "PIPE"
		if !isPipe && categoryIndex == -1 {
			description := strings.TrimSpace(row[descIndex])
			if strings.Contains(strings.ToUpper(description), "PIPE") {
				isPipe = true
			}
		}
		
		if isPipe {
			description := strings.TrimSpace(row[descIndex])
			if description == "" {
				continue
			}
			
			ns := ""
			qty := ""
			
			// Based on debug output, the actual structure is:
			// Index 0: COMPONENT DESCRIPTION
			// Index 1: QTY (with possible M suffix)
			// Index 2: WEIGHT
			// Index 5: CATEGORY
			
			// Get the description from index 0 (not descIndex which is 1)
			if len(row) > 0 {
				description = strings.TrimSpace(row[0])
			}
			
			// Get QTY from index 1 (not qtyIndex which is 3)
			if len(row) > 1 {
				qty = cleanQtyValue(strings.TrimSpace(row[1]))
			}
			
			// N.S. appears to be missing in this data structure, leave it empty for now
			// We can add logic later to extract it from the description if needed
			
			pipeEntries = append(pipeEntries, struct {
				description string
				ns         string
				qty        string
			}{description, ns, qty})
			
			debugPrint(fmt.Sprintf("[DEBUG] Fixed PIPE: desc='%s', ns='%s', qty='%s'", description, ns, qty))
			
			pipeInfo.AllPipes = append(pipeInfo.AllPipes, description)
		}
	}
	
	pipeInfo.Count = len(pipeEntries)
	
	if len(pipeEntries) > 0 {
		// Use the first pipe entry
		pipeInfo.Description = pipeEntries[0].description
		pipeInfo.NS = pipeEntries[0].ns
		pipeInfo.Qty = pipeEntries[0].qty
	}
	
	return pipeInfo
}

// cleanQtyValue removes 'M' suffix and other formatting from quantity values
func cleanQtyValue(qty string) string {
	qty = strings.TrimSpace(qty)
	if strings.HasSuffix(strings.ToUpper(qty), "M") {
		qty = strings.TrimSuffix(qty, "M")
		qty = strings.TrimSuffix(qty, "m")
		qty = strings.TrimSpace(qty)
	}
	return qty
}

// extractTable extracts table data from text entities (complete version from table_extraction.go)
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

// mergeHeaderForCutPipeLength merges header rows for cut pipe length table
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

// processErectionMaterialsTable processes the ERECTION MATERIALS table
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

			// Remove "M" from pipe length cells (QTY column - typically column D)
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

// validateAndCorrectCutLengthRow validates and corrects cut length rows (simplified version)
func validateAndCorrectCutLengthRow(row []string) []string {
	// For the weld detector, we don't need the complex cut length validation
	// Just return the row as-is
	return row
}

// TableCell represents a cell with its X position and content
type TableCell struct {
	X    float64
	Text string
}

// findPipeClass extracts pipe class from text entities using the same logic as BOM extractor
func findPipeClass(textEntities []TextEntity) string {
	// Look for pipe class patterns like "AHDX" (4 capital letters)
	// Usually found in bottom half of drawing, center-left area
	
	if len(textEntities) == 0 {
		return ""
	}
	
	// Focus on bottom half of drawing
	bottomEntities := []TextEntity{}
	if len(textEntities) > 100 {
		// Use bottom 3/4 for larger drawings
		startIndex := len(textEntities) / 4
		bottomEntities = textEntities[startIndex:]
	} else {
		// Use bottom half for smaller drawings
		midPoint := len(textEntities) / 2
		bottomEntities = textEntities[:midPoint]
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
		sort.Slice(centerCandidates, func(i, j int) bool {
			return centerCandidates[i].x < centerCandidates[j].x
		})
		pipeClass := centerCandidates[0].value
		return pipeClass
	}
	
	return ""
}

// parseTextEntities extracts text entities from DXF content for BOM extraction
func (opwd *OptimizedPolylineWeldDetector) parseTextEntities(content string) ([]TextEntity, error) {
	entities := []TextEntity{}
	
	scanner := bufio.NewScanner(strings.NewReader(content))
	
	var currentEntity *TextEntity
	var currentCode string
	
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		
		if line == "" {
			continue
		}
		
		// Check if this is a group code
		if code, err := strconv.Atoi(line); err == nil {
			currentCode = line
			
			// If we were building an entity and hit a new entity start, save the previous one
			if (code == 0 || code == 8) && currentEntity != nil && currentEntity.Content != "" {
				entities = append(entities, *currentEntity)
				currentEntity = nil
			}
			
			continue
		}
		
		// Handle different group codes
		switch currentCode {
		case "0":
			// Entity type
			if line == "TEXT" || line == "MTEXT" {
				currentEntity = &TextEntity{
					EntityType: line,
				}
			} else {
				// Save previous entity if it exists
				if currentEntity != nil && currentEntity.Content != "" {
					entities = append(entities, *currentEntity)
				}
				currentEntity = nil
			}
			
		case "1", "3":
			// Text content (group code 1 for TEXT, 3 for additional MTEXT content)
			if currentEntity != nil {
				if currentEntity.Content != "" {
					currentEntity.Content += line // Append for multi-line text
				} else {
					currentEntity.Content = line
				}
			}
			
		case "10":
			// X coordinate
			if currentEntity != nil {
				if x, err := strconv.ParseFloat(line, 64); err == nil {
					currentEntity.X = x
				}
			}
			
		case "20":
			// Y coordinate
			if currentEntity != nil {
				if y, err := strconv.ParseFloat(line, 64); err == nil {
					currentEntity.Y = y
				}
			}
			
		case "40":
			// Text height
			if currentEntity != nil {
				if height, err := strconv.ParseFloat(line, 64); err == nil {
					currentEntity.Height = height
				}
			}
			
		case "8":
			// Layer name
			if currentEntity != nil {
				currentEntity.Layer = line
			}
		}
	}
	
	// Don't forget the last entity
	if currentEntity != nil && currentEntity.Content != "" {
		entities = append(entities, *currentEntity)
	}
	
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error scanning content: %v", err)
	}
	
	return entities, nil
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
	
	if !globalQuiet {
		fmt.Printf("    Starting detection with %d target segments...\n", len(segments))
	}
	
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
	if !globalQuiet {
		fmt.Printf("    Pair checking time: %.2f seconds\n", pairTime.Seconds())
		fmt.Printf("    Pair statistics: %d total pairs, %d valid length pairs, %d intersection checks\n", 
			totalPairs, validPairs, intersectionChecks)
	}
	
	// Remove duplicates (same location)
	dedupeStart := time.Now()
	uniqueSymbols := opwd.removeDuplicateSymbols(weldSymbols)
	dedupeTime := time.Since(dedupeStart)
	if !globalQuiet {
		fmt.Printf("    Deduplication time: %.2f seconds (%d -> %d symbols)\n", 
			dedupeTime.Seconds(), len(weldSymbols), len(uniqueSymbols))
	}
	
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
	if !globalQuiet {
		fmt.Printf("Reading file: %s\n", filename)
	}
	readStart := time.Now()
	content, err := os.ReadFile(filePath)
	if err != nil {
		result.Error = fmt.Sprintf("Failed to read file: %v", err)
		result.ProcessingTime = time.Since(start).Seconds()
		return result
	}
	readTime := time.Since(readStart)
	if !globalQuiet {
		fmt.Printf("  File read time: %.2f seconds (%.1f MB)\n", readTime.Seconds(), float64(len(content))/1024/1024)
	}
	
	// Parse POLYLINE segments (optimized - only target lengths)
	parseStart := time.Now()
	segments, err := opwd.parsePolylineSegmentsOptimized(string(content))
	if err != nil {
		result.Error = fmt.Sprintf("Failed to parse POLYLINE segments: %v", err)
		result.ProcessingTime = time.Since(start).Seconds()
		return result
	}
	parseTime := time.Since(parseStart)
	if !globalQuiet {
		fmt.Printf("  Optimized parse time: %.2f seconds (%d target segments)\n", parseTime.Seconds(), len(segments))
	}
	
	// Detect weld symbols
	detectStart := time.Now()
	weldSymbols := opwd.detectWeldSymbols(segments)
	detectTime := time.Since(detectStart)
	if !globalQuiet {
		fmt.Printf("  Detection time: %.2f seconds (%d welds found)\n", detectTime.Seconds(), len(weldSymbols))
	}
	
	result.WeldCount = len(weldSymbols)
	
	// Extract pipe information from BOM
	bomStart := time.Now()
	textEntities, err := opwd.parseTextEntities(string(content))
	if err != nil {
		// Don't fail the whole process if BOM extraction fails
		fmt.Printf("  Warning: Failed to parse text entities for BOM: %v\n", err)
		result.PipeClass = ""
		result.PipeDescription = "BOM extraction failed"
		result.PipeNS = ""
		result.PipeQty = ""
		result.MultiplePipesNote = ""
	} else {
		pipeInfo := extractPipeInfoFromBOM(textEntities)
		result.PipeClass = pipeInfo.PipeClass
		result.PipeDescription = pipeInfo.Description
		result.PipeNS = pipeInfo.NS
		result.PipeQty = pipeInfo.Qty
		
		// Generate multiple pipes note if needed
		if pipeInfo.Count > 1 {
			result.MultiplePipesNote = fmt.Sprintf("First of %d PIPE components selected: %s", 
				pipeInfo.Count, strings.Join(pipeInfo.AllPipes, "; "))
		} else {
			result.MultiplePipesNote = ""
		}
		
		bomTime := time.Since(bomStart)
		if !globalQuiet {
			fmt.Printf("  BOM extraction time: %.2f seconds (%d pipe(s) found)\n", bomTime.Seconds(), pipeInfo.Count)
		}
	}
	
	result.ProcessingTime = time.Since(start).Seconds()
	
	if !globalQuiet {
		totalTime := time.Since(start)
		fmt.Printf("  Total time: %.2f seconds\n", totalTime.Seconds())
		fmt.Printf("  Breakdown: Read %.1f%%, Parse %.1f%%, Detect %.1f%%\n", 
			readTime.Seconds()/totalTime.Seconds()*100,
			parseTime.Seconds()/totalTime.Seconds()*100,
			detectTime.Seconds()/totalTime.Seconds()*100)
		fmt.Println()
	}
	
	return result
}

// processFiles processes multiple DXF files using an efficient worker pool
func (opwd *OptimizedPolylineWeldDetector) processFiles(filePaths []string, quiet bool) ([]WeldResult, error) {
	// Use the highly efficient worker pool pattern from BOM extractor
	jobs := make(chan string, len(filePaths))
	results := make(chan WeldResult, len(filePaths))
	
	// Start worker pool
	for w := 0; w < opwd.workers; w++ {
		go func() {
			for filePath := range jobs {
				result := opwd.processFile(filePath)
				results <- result
			}
		}()
	}
	
	// Send all jobs to the worker pool
	for _, filePath := range filePaths {
		jobs <- filePath
	}
	close(jobs) // Signal no more jobs
	
	// Collect results with progress reporting
	var allResults []WeldResult
	for i := 0; i < len(filePaths); i++ {
		result := <-results
		allResults = append(allResults, result)
		
		// Progress reporting (only if not quiet)
		if !quiet {
			fmt.Printf("Completed file %d/%d: %s\n", i+1, len(filePaths), filepath.Base(result.Filename))
		} else if i%100 == 0 || i == len(filePaths)-1 {
			// In quiet mode, show progress every 100 files or at the end
			fmt.Printf("Progress: %d/%d files completed\n", i+1, len(filePaths))
		}
	}
	
	return allResults, nil
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
	
	// Write header with new pipe information columns
	header := []string{"Filename", "FilePath", "WeldCount", "PipeClass", "PipeDescription", "PipeNS", "PipeQty", "MultiplePipesNote", "ProcessingTime", "Error"}
	if err := writer.Write(header); err != nil {
		return err
	}
	
	// Write data
	for _, result := range results {
		row := []string{
			result.Filename,
			result.FilePath,
			strconv.Itoa(result.WeldCount),
			result.PipeClass,
			result.PipeDescription,
			result.PipeNS,
			result.PipeQty,
			result.MultiplePipesNote,
			fmt.Sprintf("%.3f", result.ProcessingTime),
			result.Error,
		}
		if err := writer.Write(row); err != nil {
			return err
		}
	}
	
	return nil
}

// AggregatedPipeData represents aggregated pipe data across multiple files
type AggregatedPipeData struct {
	PipeDescription   string
	PipeNS           string
	TotalWeldCount   int
	TotalPipeQty     float64
	FileCount        int
	PipeClasses      []string
	Files            []string
	MultiplePipesNote string
}

// writeAggregatedResults writes aggregated results by pipe description
func writeAggregatedResults(filename string, results []WeldResult) error {
	if len(results) <= 1 {
		// Don't create aggregated file for single file processing
		return nil
	}
	
	// Group results by pipe description
	aggregationMap := make(map[string]*AggregatedPipeData)
	
	for _, result := range results {
		if result.Error != "" || result.PipeDescription == "" || result.PipeDescription == "No PIPE found" {
			continue
		}
		
		key := result.PipeDescription
		
		if agg, exists := aggregationMap[key]; exists {
			// Update existing aggregation
			agg.TotalWeldCount += result.WeldCount
			if qty, err := strconv.ParseFloat(result.PipeQty, 64); err == nil {
				agg.TotalPipeQty += qty
			}
			agg.FileCount++
			agg.Files = append(agg.Files, result.Filename)
			
			// Add pipe class if not already present
			found := false
			for _, class := range agg.PipeClasses {
				if class == result.PipeClass {
					found = true
					break
				}
			}
			if !found && result.PipeClass != "" {
				agg.PipeClasses = append(agg.PipeClasses, result.PipeClass)
			}
			
			// Keep multiple pipes note if any file had it
			if result.MultiplePipesNote != "" {
				if agg.MultiplePipesNote == "" {
					agg.MultiplePipesNote = result.MultiplePipesNote
				} else {
					agg.MultiplePipesNote += "; " + result.MultiplePipesNote
				}
			}
		} else {
			// Create new aggregation
			qty := 0.0
			if qtyVal, err := strconv.ParseFloat(result.PipeQty, 64); err == nil {
				qty = qtyVal
			}
			
			pipeClasses := []string{}
			if result.PipeClass != "" {
				pipeClasses = append(pipeClasses, result.PipeClass)
			}
			
			aggregationMap[key] = &AggregatedPipeData{
				PipeDescription:   result.PipeDescription,
				PipeNS:           result.PipeNS,
				TotalWeldCount:   result.WeldCount,
				TotalPipeQty:     qty,
				FileCount:        1,
				PipeClasses:      pipeClasses,
				Files:            []string{result.Filename},
				MultiplePipesNote: result.MultiplePipesNote,
			}
		}
	}
	
	if len(aggregationMap) == 0 {
		return nil // No data to aggregate
	}
	
	// Create aggregated CSV file
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()
	
	writer := csv.NewWriter(file)
	defer writer.Flush()
	
	// Write header
	header := []string{"PipeDescription", "PipeNS", "TotalWeldCount", "TotalPipeQty", "FileCount", "PipeClass", "MultiplePipesNote"}
	if err := writer.Write(header); err != nil {
		return err
	}
	
	// Convert map to slice for consistent ordering
	aggregations := []*AggregatedPipeData{}
	for _, agg := range aggregationMap {
		aggregations = append(aggregations, agg)
	}
	
	// Sort by pipe description for consistent output
	sort.Slice(aggregations, func(i, j int) bool {
		return aggregations[i].PipeDescription < aggregations[j].PipeDescription
	})
	
	// Write aggregated data
	for _, agg := range aggregations {
		row := []string{
			agg.PipeDescription,
			agg.PipeNS,
			strconv.Itoa(agg.TotalWeldCount),
			fmt.Sprintf("%.1f", agg.TotalPipeQty),
			strconv.Itoa(agg.FileCount),
			strings.Join(agg.PipeClasses, ";"),
			agg.MultiplePipesNote,
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
	var quiet bool
	
	flag.StringVar(&directory, "dir", "", "Directory containing DXF files")
	flag.StringVar(&filePath, "file", "", "Single DXF file to analyze")
	flag.IntVar(&workers, "workers", 0, "Number of parallel workers (default: auto)")
	flag.StringVar(&output, "output", "weld_counts.csv", "Output CSV filename")
	flag.BoolVar(&quiet, "quiet", false, "Quiet mode: minimal output for large batches")
	
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
	
	if !quiet {
		fmt.Printf("Found %d DXF files to analyze for POLYLINE weld symbols...\n", len(dxfFiles))
		fmt.Printf("Looking for crossed POLYLINE segments with length pairs:\n")
		for _, pair := range weldLengthPairs {
			fmt.Printf("  - %.4f & %.4f\n", pair[0], pair[1])
		}
		fmt.Printf("Optimized version: Filtering during parse for maximum speed\n")
		fmt.Println()
	} else {
		fmt.Printf("Processing %d DXF files...\n", len(dxfFiles))
	}
	
	// Set global quiet mode
	globalQuiet = quiet
	
	// Intelligent worker count selection (same as BOM extractor)
	if workers == 0 {
		// Auto-determine: use parallel processing for multiple files
		if len(dxfFiles) > 1 {
			workers = min(len(dxfFiles), runtime.NumCPU())
		} else {
			workers = 1
		}
	}
	
	// Report processing strategy
	if !quiet {
		if workers > 1 {
			fmt.Printf("Processing %d DXF files using %d parallel workers...\n", len(dxfFiles), workers)
		} else {
			fmt.Printf("Processing %d DXF files sequentially...\n", len(dxfFiles))
		}
		fmt.Println()
	}
	
	// Create detector
	detector := NewOptimizedPolylineWeldDetector(workers)
	start := time.Now()
	
	// Process files
	results, err := detector.processFiles(dxfFiles, quiet)
	if err != nil {
		fmt.Printf("Error processing files: %v\n", err)
		os.Exit(1)
	}
	
	// Determine output path - place CSV in the same directory where we started processing
	var outputPath string
	if directory != "" {
		// Directory mode: place output in the specified directory
		outputPath = filepath.Join(directory, output)
	} else {
		// Single file mode: place output in the directory containing the file
		outputPath = filepath.Join(filepath.Dir(filePath), output)
	}
	
	// Write results
	if err := writeResults(outputPath, results); err != nil {
		fmt.Printf("Error writing results: %v\n", err)
		os.Exit(1)
	}
	
	// Write aggregated results if multiple files processed
	if len(results) > 1 {
		aggregatedPath := filepath.Join(filepath.Dir(outputPath), "weld_counts_aggregated.csv")
		if err := writeAggregatedResults(aggregatedPath, results); err != nil {
			fmt.Printf("Error writing aggregated results: %v\n", err)
		} else {
			fmt.Printf("Aggregated results written to: %s\n", aggregatedPath)
		}
	}
	
	// Summary with enhanced parallel metrics
	totalWelds := 0
	successCount := 0
	totalProcessingTime := 0.0
	
	for _, result := range results {
		if result.Error == "" {
			totalWelds += result.WeldCount
			successCount++
			totalProcessingTime += result.ProcessingTime
		}
	}
	
	elapsed := time.Since(start)
	
	fmt.Printf("============================================================\n")
	fmt.Printf("WELD SYMBOL DETECTION COMPLETE\n")
	fmt.Printf("============================================================\n")
	fmt.Printf("Total Files: %d\n", len(results))
	fmt.Printf("Successful: %d\n", successCount)
	fmt.Printf("Failed: %d\n", len(results)-successCount)
	fmt.Printf("Workers: %d\n", workers)
	fmt.Printf("Total Weld Symbols Found: %d\n", totalWelds)
	if successCount > 0 {
		fmt.Printf("Average Welds per File: %.1f\n", float64(totalWelds)/float64(successCount))
	}
	fmt.Printf("Wall Clock Time: %.3f seconds\n", elapsed.Seconds())
	fmt.Printf("Total Processing Time: %.3f seconds\n", totalProcessingTime)
	
	// Parallel efficiency calculation (same as BOM extractor)
	if workers > 1 && totalProcessingTime > 0 && elapsed.Seconds() > 0 {
		efficiency := (totalProcessingTime / elapsed.Seconds()) * 100 / float64(workers)
		fmt.Printf("Parallel Efficiency: %.1f%%\n", efficiency)
	}
	
	if successCount > 0 {
		avgTime := totalProcessingTime / float64(successCount)
		fmt.Printf("Average Time per File: %.3f seconds\n", avgTime)
	}
	
	fmt.Printf("Output File: %s\n", outputPath)
	fmt.Printf("============================================================\n")
}
