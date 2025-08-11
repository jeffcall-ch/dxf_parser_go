package main

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

func isPieceNumber(text string) bool {
	return pieceNumberPattern.MatchString(strings.TrimSpace(text))
}

func isTextRemark(text string) bool {
	textStr := strings.TrimSpace(text)
	if textStr == "" {
		return false
	}
	return !isPieceNumber(textStr) && !isNumber(textStr)
}

func validateAndCorrectCutLengthRow(row []string) []string {
	if len(row) == 0 {
		return row
	}

	// Create an 8-column template
	corrected := make([]string, 8)

	// Find piece numbers first to establish groupings
	pieces := []struct {
		index int
		value string
	}{}
	
	for i, cell := range row {
		cellStr := strings.TrimSpace(cell)
		if isPieceNumber(cellStr) {
			pieces = append(pieces, struct {
				index int
				value string
			}{i, cellStr})
		}
	}

	debugPrint(fmt.Sprintf("[DEBUG] Row validation - Found pieces: %v", pieces))

	if len(pieces) == 0 {
		// No pieces found, return original row padded/truncated to 8 columns
		for i := 0; i < 8 && i < len(row); i++ {
			corrected[i] = row[i]
		}
		return corrected
	}

	// Group data by piece based on original positions
	type pieceGroup struct {
		piece   string
		numbers []string
		remarks []string
	}

	pieceGroups := []pieceGroup{}

	for pieceIdx, piece := range pieces {
		group := pieceGroup{
			piece:   piece.value,
			numbers: []string{},
			remarks: []string{},
		}

		// Determine the range for this piece
		endPos := len(row)
		if pieceIdx+1 < len(pieces) {
			endPos = pieces[pieceIdx+1].index
		}

		// Collect numbers and remarks between this piece and the next
		for i := piece.index + 1; i < endPos && i < len(row); i++ {
			cellStr := strings.TrimSpace(row[i])
			if cellStr == "" {
				continue
			}

			if isNumber(cellStr) {
				group.numbers = append(group.numbers, cellStr)
			} else if isTextRemark(cellStr) {
				group.remarks = append(group.remarks, cellStr)
			}
		}

		pieceGroups = append(pieceGroups, group)
	}

	debugPrint(fmt.Sprintf("[DEBUG] Piece groups: %v", pieceGroups))

	// Fill the corrected row
	maxPieces := 2
	if len(pieceGroups) < maxPieces {
		maxPieces = len(pieceGroups)
	}

	for groupIdx := 0; groupIdx < maxPieces; groupIdx++ {
		group := pieceGroups[groupIdx]
		baseCol := groupIdx * 4 // 0 for first piece, 4 for second piece

		// Place piece number
		corrected[baseCol] = group.piece

		// Place numbers (should be CUT LENGTH and N.S.)
		for numIdx := 0; numIdx < 2 && numIdx < len(group.numbers); numIdx++ {
			corrected[baseCol+1+numIdx] = group.numbers[numIdx]
		}

		// Place remarks
		if len(group.remarks) > 0 {
			corrected[baseCol+3] = group.remarks[0] // First remark only
		}
	}

	debugPrint(fmt.Sprintf("[DEBUG] Corrected row: %v", corrected))
	return corrected
}

func findPipeClass(textEntities []TextEntity) string {
	// Look for 'Pipe class:' label first
	var pipeClassLabelY, pipeClassLabelX *float64

	for _, entity := range textEntities {
		textClean := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(entity.Content, " ", ""), "\n", ""))
		// More flexible matching for pipe class label
		if strings.Contains(textClean, "pipeclass") ||
			strings.Contains(textClean, "pipe_class") ||
			(strings.Contains(strings.ToLower(entity.Content), "pipe") && strings.Contains(strings.ToLower(entity.Content), "class")) {
			pipeClassLabelX = &entity.X
			pipeClassLabelY = &entity.Y
			debugPrint(fmt.Sprintf("[DEBUG] Found pipe class label at X=%f, Y=%f: '%s'", entity.X, entity.Y, entity.Content))
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

		for _, entity := range textEntities {
			// Look for text near the label (horizontally close, similar Y level)
			if abs(entity.Y-*pipeClassLabelY) < 20 && // Same row or close
				entity.X > *pipeClassLabelX && // To the right of label
				abs(entity.X-*pipeClassLabelX) < 200 { // Not too far horizontally
				textClean := strings.TrimSpace(entity.Content)
				match := pipeClassPattern.FindString(textClean)
				if match != "" {
					distance := abs(entity.X - *pipeClassLabelX)
					candidates = append(candidates, candidate{match, distance})
					debugPrint(fmt.Sprintf("[DEBUG] Pipe class candidate: '%s' at distance %f", match, distance))
				}
			}
		}

		if len(candidates) > 0 {
			// Sort by distance from label and pick the closest one
			sort.Slice(candidates, func(i, j int) bool {
				return candidates[i].distance < candidates[j].distance
			})
			pipeClass := candidates[0].value
			debugPrint(fmt.Sprintf("[DEBUG] Selected pipe class: '%s'", pipeClass))
			return pipeClass
		}
	}

	// Alternative approach: Look for DESIGN DATA section first, then find pipe class within it
	var designDataY *float64
	for _, entity := range textEntities {
		if strings.Contains(strings.ToUpper(entity.Content), "DESIGN DATA") {
			designDataY = &entity.Y
			debugPrint(fmt.Sprintf("[DEBUG] Found DESIGN DATA at Y=%f", entity.Y))
			break
		}
	}

	if designDataY != nil {
		// Look for 4-letter codes within DESIGN DATA area (below the title)
		for _, entity := range textEntities {
			if entity.Y < *designDataY && entity.Y > *designDataY-150 { // Within 150 units below DESIGN DATA
				textClean := strings.TrimSpace(entity.Content)
				match := pipeClassPattern.FindString(textClean)
				if match != "" {
					pipeClass := match
					debugPrint(fmt.Sprintf("[DEBUG] Found pipe class in DESIGN DATA area: '%s' at X=%f, Y=%f", pipeClass, entity.X, entity.Y))
					return pipeClass
				}
			}
		}
	}

	// Fallback: look for 4-letter codes in bottom center area
	bottomEntities := []TextEntity{}
	for _, entity := range textEntities {
		if entity.Y < 100 { // Y < 100 (bottom area)
			bottomEntities = append(bottomEntities, entity)
		}
	}

	if len(bottomEntities) == 0 {
		// Use bottom half if no entities found below Y=100
		sort.Slice(textEntities, func(i, j int) bool {
			return textEntities[i].Y < textEntities[j].Y
		})
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
				debugPrint(fmt.Sprintf("[DEBUG] Center area pipe class candidate: '%s' at X=%f, Y=%f", match, entity.X, entity.Y))
			}
		}
	}

	if len(centerCandidates) > 0 {
		// Prefer candidates in the center-left area (where DESIGN DATA typically is)
		sort.Slice(centerCandidates, func(i, j int) bool {
			return centerCandidates[i].x < centerCandidates[j].x
		})
		pipeClass := centerCandidates[0].value
		debugPrint(fmt.Sprintf("[DEBUG] Selected center area pipe class: '%s'", pipeClass))
		return pipeClass
	}

	debugPrint("[DEBUG] No pipe class found")
	return ""
}

func findDrawingNo(textEntities []TextEntity) string {
	// Find KKS code with pattern 1AAA11BR111 (1=digit, A=capital letter, BR=fixed)
	// Located in bottom right corner, below and to the right of ERECTION MATERIALS

	// First find ERECTION MATERIALS position to establish search area
	var erectionX, erectionY *float64
	for _, entity := range textEntities {
		if strings.Contains(strings.ToUpper(entity.Content), "ERECTION MATERIALS") {
			erectionX = &entity.X
			erectionY = &entity.Y
			debugPrint(fmt.Sprintf("[DEBUG] Found ERECTION MATERIALS at X=%f, Y=%f", entity.X, entity.Y))
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

	for _, entity := range textEntities {
		match := kksPattern.FindString(entity.Content)
		if match != "" {
			// If we found ERECTION MATERIALS, filter by position (below and to the right)
			if erectionX != nil && erectionY != nil {
				if entity.X >= *erectionX && entity.Y <= *erectionY { // Right and below
					candidates = append(candidates, candidate{match, entity.X, entity.Y, abs(entity.Y)})
					debugPrint(fmt.Sprintf("[DEBUG] Found KKS candidate '%s' at X=%f, Y=%f", match, entity.X, entity.Y))
				}
			} else {
				// If no ERECTION MATERIALS found, consider all KKS codes
				candidates = append(candidates, candidate{match, entity.X, entity.Y, abs(entity.Y)})
				debugPrint(fmt.Sprintf("[DEBUG] Found KKS candidate '%s' at X=%f, Y=%f (no ERECTION MATERIALS reference)", match, entity.X, entity.Y))
			}
		}
	}

	if len(candidates) > 0 {
		// Sort by Y coordinate (lowest Y = bottom of drawing) and pick the first one
		sort.Slice(candidates, func(i, j int) bool {
			return candidates[i].absY < candidates[j].absY
		})
		selectedKKS := candidates[0].value
		debugPrint(fmt.Sprintf("[DEBUG] Selected KKS code: '%s'", selectedKKS))
		return selectedKKS
	}

	// Fallback: try to find Drawing-No. field if no KKS found
	for i, entity := range textEntities {
		if strings.Contains(entity.Content, "Drawing-No.") {
			// Look for next text entity to the right or below
			for j := i + 1; j < len(textEntities) && j < i+5; j++ {
				nextEntity := textEntities[j]
				if nextEntity.X > entity.X || nextEntity.Y < entity.Y {
					debugPrint(fmt.Sprintf("[DEBUG] Fallback: Found Drawing-No. '%s'", nextEntity.Content))
					return nextEntity.Content
				}
			}
		}
	}

	debugPrint("[DEBUG] No KKS code or Drawing-No. found")
	return ""
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// Extract pipe descriptions from material table (PIPE category only)
func extractPipeDescriptions(matRows [][]string) []string {
	var pipeDescriptions []string
	
	for _, row := range matRows {
		if len(row) >= 6 && row[5] == "PIPE" { // Category is in column F (index 5)
			if len(row) >= 2 && row[1] != "" { // Description is in column B (index 1)
				pipeDescriptions = append(pipeDescriptions, row[1])
			}
		}
	}
	
	return pipeDescriptions
}

func convertCutLengthToSingleRowFormat(header []string, rows [][]string, drawingNo, pipeClass string, pipeDescriptions []string) ([]string, [][]string) {
	if len(rows) == 0 {
		return []string{"PIECE NO", "CUT LENGTH", "N.S. (MM)", "REMARKS", "PIPE DESCRIPTION", "MULTIPLE PIPE DESCRIPTIONS", "Drawing-No.", "Pipe Class"}, [][]string{}
	}

	// Determine pipe description to use
	pipeDesc := ""
	multipleDesc := "NO"
	if len(pipeDescriptions) == 0 {
		pipeDesc = "No pipe description found"
	} else if len(pipeDescriptions) == 1 {
		pipeDesc = pipeDescriptions[0]
	} else {
		// Join all pipe descriptions with " | " separator
		pipeDesc = strings.Join(pipeDescriptions, " | ")
		multipleDesc = "YES"
	}

	// New header format with pipe description and multiple flag
	newHeader := []string{"PIECE NO", "CUT LENGTH", "N.S. (MM)", "REMARKS", "PIPE DESCRIPTION", "MULTIPLE PIPE DESCRIPTIONS", "Drawing-No.", "Pipe Class"}
	newRows := [][]string{}

	for _, row := range rows {
		// Extract first piece (columns 0-3)
		if len(row) >= 4 {
			piece1No := ""
			piece1Length := ""
			piece1NS := ""
			piece1Remarks := ""

			if len(row) > 0 {
				piece1No = strings.TrimSpace(row[0])
			}
			if len(row) > 1 {
				piece1Length = strings.TrimSpace(row[1])
			}
			if len(row) > 2 {
				piece1NS = strings.TrimSpace(row[2])
			}
			if len(row) > 3 {
				piece1Remarks = strings.TrimSpace(row[3])
			}

			if piece1No != "" { // Only add if piece number exists
				newRows = append(newRows, []string{piece1No, piece1Length, piece1NS, piece1Remarks, pipeDesc, multipleDesc, drawingNo, pipeClass})
			}
		}

		// Extract second piece (columns 4-7)
		if len(row) >= 8 {
			piece2No := ""
			piece2Length := ""
			piece2NS := ""
			piece2Remarks := ""

			if len(row) > 4 {
				piece2No = strings.TrimSpace(row[4])
			}
			if len(row) > 5 {
				piece2Length = strings.TrimSpace(row[5])
			}
			if len(row) > 6 {
				piece2NS = strings.TrimSpace(row[6])
			}
			if len(row) > 7 {
				piece2Remarks = strings.TrimSpace(row[7])
			}

			if piece2No != "" { // Only add if piece number exists
				newRows = append(newRows, []string{piece2No, piece2Length, piece2NS, piece2Remarks, pipeDesc, multipleDesc, drawingNo, pipeClass})
			}
		}
	}

	return newHeader, newRows
}

func processDXFFile(filepath string) DXFResult {
	start := time.Now()
	result := DXFResult{
		Filename: filepath,
		FilePath: filepath,
	}

	debugPrint(fmt.Sprintf("[DEBUG] Opening DXF file: %s", filepath))

	// Use our existing Go DXF parser
	parser := NewDXFParser(1) // Use single worker for individual file processing
	textEntities, err := parser.ParseFile(filepath)
	if err != nil {
		result.Error = fmt.Sprintf("Failed to parse DXF file: %v", err)
		result.ProcessingTime = time.Since(start).Seconds()
		return result
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

	return result
}
