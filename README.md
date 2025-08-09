# DXF Text Parser

A high-performance Go program for extracting text content and coordinates from DXF (Drawing Exchange Format) files. Optimized for speed with concurrent processing and designed to handle large files (12MB+) efficiently.

## Features

- **Fast Text Extraction**: Extracts all TEXT and MTEXT entities from DXF files
- **Coordinate Capture**: Captures precise X,Y coordinates and text height for each entity
- **Concurrent Processing**: Utilizes multiple CPU cores for optimal performance
- **Spatial Analysis**: Advanced spatial queries and position-based analysis
- **Memory Efficient**: Stream processing to handle large files without excessive memory usage
- **Command-Line Interface**: Easy-to-use CLI with multiple operation modes

## Performance

- Processes 12MB+ DXF files in under 2 seconds
- Scales efficiently across multiple CPU cores
- Memory-optimized for handling large technical drawings

## Installation

```bash
# Clone the repository
git clone https://github.com/jeffcall-ch/dxf_parser_go.git
cd dxf_parser_go

# Build the executable
go build -o dxf_parser.exe
```

## Usage

### Basic Parsing

Extract all text entities from a DXF file:

```bash
# Parse with default number of workers (CPU cores)
./dxf_parser parse drawing.dxf

# Parse with specific number of workers
./dxf_parser parse drawing.dxf 8
```

### Spatial Analysis

Perform spatial analysis on extracted text entities:

```bash
# Show statistics about the drawing
./dxf_parser spatial drawing.dxf stats

# Find entities near specific text
./dxf_parser spatial drawing.dxf near "PIPE" 50.0

# Find entities in coordinate range
./dxf_parser spatial drawing.dxf range 100 100 500 500

# Find entities in top-right quadrant relative to text
./dxf_parser spatial drawing.dxf quadrant "REFERENCE_POINT"
```

### Performance Benchmarking

Test parsing performance with different worker configurations:

```bash
./dxf_parser benchmark drawing.dxf
```

## API Reference

### Core Types

```go
// TextEntity represents a text entity extracted from a DXF file
type TextEntity struct {
    Content    string  `json:"content"`      // Text content
    X          float64 `json:"x"`            // X coordinate
    Y          float64 `json:"y"`            // Y coordinate  
    Height     float64 `json:"height"`       // Text height
    EntityType string  `json:"entity_type"`  // "TEXT" or "MTEXT"
    Layer      string  `json:"layer"`        // DXF layer name
}
```

### DXF Parser

```go
// Create a new parser with specified number of workers
parser := NewDXFParser(workers)

// Parse a DXF file and extract all text entities
entities, err := parser.ParseFile("drawing.dxf")
```

### Spatial Analyzer

```go
// Create spatial analyzer with parsed entities
analyzer := NewSpatialAnalyzer(entities)

// Find entities within coordinate range
rangeEntities := analyzer.FindEntitiesInRange(minX, minY, maxX, maxY)

// Find entities within radius of a point
radiusEntities := analyzer.FindEntitiesInRadius(centerX, centerY, radius)

// Find nearest N entities to a point
nearest := analyzer.FindNearestEntities(x, y, n)

// Find entities near text containing specific string
nearEntities := analyzer.FindEntitiesNearText("PIPE", maxDistance)

// Get entities in specific quadrant relative to reference point
quadrantEntities := analyzer.GetQuadrant(refX, refY, quadrant)

// Get statistical information
stats := analyzer.GetEntityStats()
```

## Supported DXF Elements

The parser extracts the following DXF group codes:

- **Group 0**: Entity type identifier (TEXT/MTEXT)
- **Group 1**: Primary text content
- **Group 3**: Additional text content (for MTEXT continuation)
- **Group 8**: Layer name
- **Group 10**: X coordinate
- **Group 20**: Y coordinate  
- **Group 40**: Text height

## Examples

### Example 1: Basic Text Extraction

```go
package main

import (
    "fmt"
    "log"
)

func main() {
    parser := NewDXFParser(8) // Use 8 workers
    
    entities, err := parser.ParseFile("technical_drawing.dxf")
    if err != nil {
        log.Fatal(err)
    }
    
    fmt.Printf("Found %d text entities\n", len(entities))
    
    for _, entity := range entities {
        fmt.Printf("Text: %s at (%.2f, %.2f)\n", 
            entity.Content, entity.X, entity.Y)
    }
}
```

### Example 2: Spatial Queries

```go
analyzer := NewSpatialAnalyzer(entities)

// Find all text near "VALVE" annotations
nearValves := analyzer.FindEntitiesNearText("VALVE", 25.0)

// Get drawing statistics
stats := analyzer.GetEntityStats()
fmt.Printf("Drawing size: %.1f x %.1f\n", 
    stats["drawing_width"], stats["drawing_height"])

// Find text in top-right area of drawing
bbox := analyzer.GetBoundingBox()
topRight := analyzer.FindEntitiesInRange(
    bbox.MaxX*0.7, bbox.MaxY*0.7, 
    bbox.MaxX, bbox.MaxY,
)
```

## Testing

Run the test suite:

```bash
# Run all tests
go test

# Run tests with verbose output
go test -v

# Run benchmarks
go test -bench=.

# Run tests with coverage
go test -cover
```

## Performance Benchmarks

Tested on sample piping isometric drawings (~10.7MB each):

| Workers | File Size | Parse Time | Speedup |
|---------|-----------|------------|---------|
| 1       | 10.7 MB   | 1.45s     | 1.0x    |
| 2       | 10.7 MB   | 0.82s     | 1.77x   |
| 4       | 10.7 MB   | 0.51s     | 2.84x   |
| 8       | 10.7 MB   | 0.38s     | 3.82x   |

## Architecture

### Concurrent Processing

The parser uses a work-stealing approach:

1. **File Chunking**: Large files are divided into chunks at entity boundaries
2. **Worker Goroutines**: Multiple goroutines process chunks concurrently  
3. **Result Aggregation**: Results are collected and merged safely
4. **Memory Management**: Streaming approach minimizes memory usage

### Spatial Indexing

Spatial queries are optimized for technical drawings:

- **Bounding Box Queries**: Fast rectangular region searches
- **Radius Searches**: Efficient circular region queries
- **Nearest Neighbor**: K-nearest neighbor searches with distance sorting
- **Quadrant Analysis**: Relative positioning analysis

## Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Acknowledgments

- Built for efficient processing of technical CAD drawings
- Optimized for piping and instrumentation diagrams (P&IDs)
- Designed to handle large-scale engineering documentation