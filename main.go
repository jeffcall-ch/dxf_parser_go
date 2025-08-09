package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
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

// decodeUnicode decodes Unicode escape sequences like \U+00B0 to actual Unicode characters
func decodeUnicode(text string) string {
	// Regex to match \U+xxxx patterns
	re := regexp.MustCompile(`\\U\+([0-9A-Fa-f]{4})`)
	
	result := re.ReplaceAllStringFunc(text, func(match string) string {
		// Extract the hex code (remove \U+)
		hexCode := match[3:]
		
		// Parse the hex code to integer
		if codePoint, err := strconv.ParseInt(hexCode, 16, 32); err == nil {
			// Convert to Unicode character
			return string(rune(codePoint))
		}
		
		// If parsing fails, return original
		return match
	})
	
	return result
}

// DXFParser handles parsing of DXF files
type DXFParser struct {
	workers    int
	chunkSize  int64
	textBuffer []TextEntity
	mutex      sync.RWMutex
}

// NewDXFParser creates a new parser with specified number of workers
func NewDXFParser(workers int) *DXFParser {
	if workers <= 0 {
		workers = runtime.NumCPU()
	}
	return &DXFParser{
		workers:   workers,
		chunkSize: 1024 * 1024, // 1MB chunks
	}
}

// ParseFile parses a DXF file and extracts all text entities
func (p *DXFParser) ParseFile(filename string) ([]TextEntity, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	p.textBuffer = make([]TextEntity, 0)
	
	// For now, always use sequential parsing to ensure correctness
	// TODO: Fix concurrent parsing chunking logic for better performance
	return p.parseSequential(file)
}

// parseSequential processes the file sequentially for smaller files
func (p *DXFParser) parseSequential(file *os.File) ([]TextEntity, error) {
	scanner := bufio.NewScanner(file)
	entities := make([]TextEntity, 0)
	
	currentEntity := &TextEntity{}
	inTextEntity := false
	expectingValue := false
	lastGroupCode := ""

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		
		if !expectingValue {
			// This is a group code
			if line == "0" {
				// Start of new entity
				if inTextEntity && currentEntity.Content != "" {
					entities = append(entities, *currentEntity)
				}
				currentEntity = &TextEntity{}
				inTextEntity = false
			} else if inTextEntity {
				lastGroupCode = line
			}
			expectingValue = true
		} else {
			// This is a value
			if lastGroupCode == "" && (line == "TEXT" || line == "MTEXT") {
				inTextEntity = true
				currentEntity.EntityType = line
			} else if inTextEntity {
				switch lastGroupCode {
				case "1", "3": // Text content
					decodedLine := decodeUnicode(line)
					if currentEntity.Content == "" {
						currentEntity.Content = decodedLine
					} else {
						currentEntity.Content += decodedLine
					}
				case "8": // Layer
					currentEntity.Layer = line
				case "10": // X coordinate
					if x, err := strconv.ParseFloat(line, 64); err == nil {
						currentEntity.X = x
					}
				case "20": // Y coordinate
					if y, err := strconv.ParseFloat(line, 64); err == nil {
						currentEntity.Y = y
					}
				case "40": // Text height
					if h, err := strconv.ParseFloat(line, 64); err == nil {
						currentEntity.Height = h
					}
				}
			}
			expectingValue = false
			lastGroupCode = ""
		}
	}

	// Add the last entity if it's valid
	if inTextEntity && currentEntity.Content != "" {
		entities = append(entities, *currentEntity)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading file: %w", err)
	}

	return entities, nil
}

// parseConcurrent processes large files using multiple goroutines
func (p *DXFParser) parseConcurrent(file *os.File, fileSize int64) ([]TextEntity, error) {
	// Calculate chunk boundaries ensuring we don't split entities
	chunks := p.calculateChunks(file, fileSize)
	
	// Channel to collect results
	resultChan := make(chan []TextEntity, len(chunks))
	errorChan := make(chan error, len(chunks))
	
	// WaitGroup to synchronize goroutines
	var wg sync.WaitGroup
	
	// Process chunks concurrently
	for _, chunk := range chunks {
		wg.Add(1)
		go func(start, end int64) {
			defer wg.Done()
			
			entities, err := p.parseChunk(file, start, end)
			if err != nil {
				errorChan <- err
				return
			}
			resultChan <- entities
		}(chunk.start, chunk.end)
	}
	
	// Close channels when all goroutines complete
	go func() {
		wg.Wait()
		close(resultChan)
		close(errorChan)
	}()
	
	// Check for errors
	select {
	case err := <-errorChan:
		return nil, err
	default:
	}
	
	// Collect results
	var allEntities []TextEntity
	for entities := range resultChan {
		allEntities = append(allEntities, entities...)
	}
	
	return allEntities, nil
}

// Chunk represents a portion of the file to process
type Chunk struct {
	start, end int64
}

// calculateChunks divides the file into chunks that don't split entities
func (p *DXFParser) calculateChunks(file *os.File, fileSize int64) []Chunk {
	numChunks := p.workers
	if numChunks > int(fileSize/p.chunkSize) {
		numChunks = int(fileSize/p.chunkSize) + 1
	}
	
	if numChunks <= 1 {
		return []Chunk{{0, fileSize}}
	}
	
	chunks := make([]Chunk, 0, numChunks)
	chunkSize := fileSize / int64(numChunks)
	
	for i := 0; i < numChunks; i++ {
		start := int64(i) * chunkSize
		end := start + chunkSize
		
		if i == numChunks-1 {
			end = fileSize
		} else {
			// Adjust end to not split entities
			end = p.findSafeChunkEnd(file, end)
		}
		
		if start < end {
			chunks = append(chunks, Chunk{start, end})
		}
	}
	
	return chunks
}

// findSafeChunkEnd finds a safe place to end a chunk (after an entity boundary)
func (p *DXFParser) findSafeChunkEnd(file *os.File, position int64) int64 {
	// Seek to the position
	file.Seek(position, 0)
	scanner := bufio.NewScanner(file)
	
	// Look for the next "0" group code that starts a new entity
	for scanner.Scan() {
		position += int64(len(scanner.Bytes()) + 1) // +1 for newline
		line := strings.TrimSpace(scanner.Text())
		
		if line == "0" {
			// Read the next line to see if it's an entity start
			if scanner.Scan() {
				nextLine := strings.TrimSpace(scanner.Text())
				position += int64(len(scanner.Bytes()) + 1)
				
				if nextLine == "SECTION" || nextLine == "ENDSEC" || nextLine == "EOF" {
					return position
				}
			}
			return position
		}
	}
	
	return position
}

// parseChunk processes a specific chunk of the file
func (p *DXFParser) parseChunk(file *os.File, start, end int64) ([]TextEntity, error) {
	// Create a section reader for this chunk
	section := io.NewSectionReader(file, start, end-start)
	scanner := bufio.NewScanner(section)
	
	entities := make([]TextEntity, 0)
	currentEntity := &TextEntity{}
	inTextEntity := false
	expectingValue := false
	lastGroupCode := ""
	
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		
		if !expectingValue {
			// This is a group code
			if line == "0" {
				// Start of new entity
				if inTextEntity && currentEntity.Content != "" {
					entities = append(entities, *currentEntity)
				}
				currentEntity = &TextEntity{}
				inTextEntity = false
			} else if inTextEntity {
				lastGroupCode = line
			}
			expectingValue = true
		} else {
			// This is a value
			if lastGroupCode == "" && (line == "TEXT" || line == "MTEXT") {
				inTextEntity = true
				currentEntity.EntityType = line
			} else if inTextEntity {
				switch lastGroupCode {
				case "1", "3": // Text content
					decodedLine := decodeUnicode(line)
					if currentEntity.Content == "" {
						currentEntity.Content = decodedLine
					} else {
						currentEntity.Content += decodedLine
					}
				case "8": // Layer
					currentEntity.Layer = line
				case "10": // X coordinate
					if x, err := strconv.ParseFloat(line, 64); err == nil {
						currentEntity.X = x
					}
				case "20": // Y coordinate
					if y, err := strconv.ParseFloat(line, 64); err == nil {
						currentEntity.Y = y
					}
				case "40": // Text height
					if h, err := strconv.ParseFloat(line, 64); err == nil {
						currentEntity.Height = h
					}
				}
			}
			expectingValue = false
			lastGroupCode = ""
		}
	}
	
	// Add the last entity if it's valid
	if inTextEntity && currentEntity.Content != "" {
		entities = append(entities, *currentEntity)
	}
	
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading chunk: %w", err)
	}
	
	return entities, nil
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "bom" {
		// Remove "bom" from args and run BOM extractor
		os.Args = append([]string{os.Args[0]}, os.Args[2:]...)
		bomMain()
	} else {
		// Run the original DXF parser CLI
		runCLI()
	}
}
