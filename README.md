# DXF Isometric Pipe Drawing Analyzer

A high-performance Go toolkit for analyzing DXF (Drawing Exchange Format) isometric pipe drawings. Specialized for extracting pipe components, weld symbols, and generating BOMs (Bill of Materials) from technical drawings.

## Features

### 🔧 **Cut Length Extractor & BOM Generator**
- **Pipe Component Extraction**: Automatically identifies pipes, fittings, valves, and flanges
- **Cut Length Calculation**: Calculates precise pipe cut lengths from isometric drawings
- **BOM Generation**: Creates comprehensive Bill of Materials with quantities and specifications
- **Table Recognition**: Extracts data from DXF table entities
- **Spatial Analysis**: Advanced coordinate-based component positioning

### ⚡ **Weld Symbol Detection & Integration**
- **Integrated Workflow**: Weld detection combined with BOM extraction in single command
- **Precision Detection**: Identifies weld symbols as crossed POLYLINE segments
- **Length-Based Recognition**: Uses specific polyline lengths (4.0311 & 6.9462, 6.8964 & 3.9446, 6.9000 & 4.0000)
- **Intersection Analysis**: Detects properly crossed lines indicating weld locations
- **Enhanced CSV Output**: Enriched with pipe information from BOM data
- **Performance Caching**: Reuses parsed DXF data for both BOM and weld analysis
- **High Accuracy**: 100% match with manual verification on test drawings

### 🚀 **Performance**
- **Ultra-Fast Processing**: 0.08 seconds average per 10MB+ DXF file
- **Concurrent Processing**: Utilizes multiple CPU cores for batch operations
- **Memory Efficient**: Stream processing for large technical drawings
- **Batch Processing**: Handles multiple files simultaneously

## Quick Start Guide

### For New Users

1. **Download and Build**:
   ```bash
   git clone https://github.com/jeffcall-ch/dxf_parser_go.git
   cd dxf_parser_go
   go build -o bom_cut_length_extractor.exe .
   ```

2. **Basic BOM Extraction**:
   ```bash
   # Extract BOM from all DXF files in a folder
   ./bom_cut_length_extractor.exe bom -dir "path/to/dxf/files"
   ```

3. **Enhanced Analysis with Weld Detection**:
   ```bash
   # Get BOM + detailed weld analysis
   ./bom_cut_length_extractor.exe bom -dir "path/to/dxf/files" -weld
   ```

### Output Files Explained

| File | Description | Key Columns |
|------|-------------|-------------|
| `0001_ERECTION_MATERIALS.csv` | Complete materials list | Item, Description, Quantity, Unit |
| `0002_CUT_PIPE_LENGTH.csv` | Pipe cutting information | PieceNumber, Length, Diameter |
| `0003_AGGREGATED_MATERIALS.csv` | Summary by material type | Category, TotalQuantity |
| `0004_SUMMARY.csv` | Processing statistics | FileName, ProcessingTime, Status |
| `0005_WELD_COUNTS.csv` | Enhanced weld analysis | WeldCount, PipeNS, PipeDescription, MultiplePipeNS |

### Performance Tips

- **Multi-file processing**: Always use directory mode for better performance
- **Worker optimization**: System auto-detects optimal worker count
- **Debug mode**: Use `-debug` flag to troubleshoot specific files
- **Memory usage**: For very large batches, process in smaller chunks

## Installation

```bash
# Clone the repository
git clone https://github.com/jeffcall-ch/dxf_parser_go.git
cd dxf_parser_go

# Build all tools using the provided script
./build.bat
```

## Usage

### Unified BOM and Cut Length Extraction

Extract pipe components, cut lengths, and generate comprehensive BOM:

```bash
# Process all DXF files in a directory (recommended)
./bom_cut_length_extractor.exe bom -dir drawings_folder

# Process with debug output
./bom_cut_length_extractor.exe bom -dir drawings_folder -debug

# Process with custom worker count
./bom_cut_length_extractor.exe bom -dir drawings_folder -workers 8

# Process with integrated weld detection (NEW!)
./bom_cut_length_extractor.exe bom -dir drawings_folder -weld

# Process with weld detection and debug output
./bom_cut_length_extractor.exe bom -dir drawings_folder -weld -debug

# Legacy single file parsing
./bom_cut_length_extractor.exe parse single_drawing.dxf
```

**Standard Output Files:**
- `0001_ERECTION_MATERIALS.csv` - Complete materials list with descriptions
- `0002_CUT_PIPE_LENGTH.csv` - Pipe cut lengths with piece numbers
- `0003_AGGREGATED_MATERIALS.csv` - Summarized materials by type
- `0004_SUMMARY.csv` - Processing summary and statistics

**Weld Detection Output (when using -weld flag):**
- `0005_WELD_COUNTS.csv` - Enhanced weld analysis with pipe information

### Weld Symbol Detection

Count weld symbols in isometric pipe drawings:

```bash
# Integrated with BOM extractor (RECOMMENDED)
./bom_cut_length_extractor.exe bom -dir drawings_folder -weld

# Standalone weld detection
./weld_detector.exe -file drawing.dxf
./weld_detector.exe -dir drawings_folder

# Custom output file
./weld_detector.exe -file drawing.dxf -output my_results.csv
```

**Enhanced Weld Output (0005_WELD_COUNTS.csv) includes:**
- **FilePath**: Full path to processed DXF file
- **FileName**: Base filename without extension
- **DrawingNo**: Extracted drawing number (KKS code)
- **PipeClass**: Pipe classification from BOM data
- **PipeNS**: Sorted, comma-separated pipe nominal sizes (e.g., "15, 25")
- **PipeDescription**: Full pipe descriptions from BOM data
- **MultiplePipeNS**: "Yes" when multiple pipe sizes detected, empty otherwise
- **WeldCount**: Number of weld symbols detected
- **ProcessingTime**: Time taken to process the file
- **Error**: Any processing errors encountered
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

## Enhanced Weld Analysis Output

The integrated weld detection system produces a comprehensive CSV file (`0005_WELD_COUNTS.csv`) with enriched pipe information extracted from BOM data:

### Sample Output

```csv
FilePath,FileName,DrawingNo,PipeClass,PipeNS,PipeDescription,MultiplePipeNS,WeldCount,ProcessingTime,Error
drawings/TB020-INOV-2HTX67BR910_1.0.dxf,,2HTX67BR910,AHDX,"25, 40","Pipe sml. ASME-B36.19M, 1"", Sch-10S A312-TP316L, Pipe sml. ASME-B36.19M, 1-1/2"", Sch-10S A312-TP316L",Yes,12,0.204,
```

### Column Descriptions

- **PipeNS**: Sorted, comma-separated pipe nominal sizes found in the drawing
- **PipeDescription**: Complete pipe descriptions from BOM data  
- **MultiplePipeNS**: "Yes" indicates multiple pipe sizes (useful for complex drawings)
- **WeldCount**: Precise count of weld symbols using geometric detection
- **DrawingNo**: Automatically extracted KKS/drawing number
- **PipeClass**: Pipe classification system identifier

### Integration Benefits

✅ **Single Command**: BOM extraction + weld detection in one operation  
✅ **Data Correlation**: Weld counts linked to specific pipe information  
✅ **Performance**: ~2x faster than running tools separately  
✅ **Enhanced Analysis**: Identify drawings with multiple pipe sizes  
✅ **Quality Control**: Cross-reference weld counts with pipe complexity

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

## BOM Extractor (Python Migration)

This tool also includes a specialized BOM (Bill of Materials) extractor that migrates the functionality of the original Python DXF processing workflow to Go, providing significant performance improvements while maintaining identical output format.

### Features

- **Automated Table Extraction**: Extracts ERECTION MATERIALS and CUT PIPE LENGTH tables from isometric drawings
- **Metadata Extraction**: Automatically identifies drawing numbers, pipe classes, and KKS codes
- **Parallel Processing**: Processes multiple DXF files concurrently with configurable worker pools
- **CSV Output**: Generates structured CSV files compatible with existing workflows
- **Error Handling**: Comprehensive validation and error reporting
- **Performance Optimization**: ~3-4x faster than the original Python implementation

### Usage

Process all DXF files in a directory:

```bash
# Basic usage
./dxf_parser bom -dir /path/to/dxf/files

# With debug output for detailed logging
./dxf_parser bom -dir /path/to/dxf/files -debug

# With custom worker count
./dxf_parser bom -dir /path/to/dxf/files -workers 8
```

### Output Files

The BOM extractor generates three CSV files:

1. **0001_ERECTION_MATERIALS.csv** - All extracted material data with metadata
2. **0002_CUT_PIPE_LENGTH.csv** - All cut pipe length data with piece information
3. **0003_SUMMARY.csv** - Processing summary with file metadata and statistics

### Performance Results

Tested on engineering isometric drawings:

- **Processing Speed**: ~380-400ms per file
- **Parallel Efficiency**: 93%+ with multiple workers  
- **Throughput**: 4 files processed in 0.41 seconds (wall clock time)
- **Speedup**: 3.7x speedup over sequential processing

Example output:
```
============================================================
PROCESSING COMPLETE
============================================================
Directory: ./dxf_test_input_files
Total Files: 4
Successful: 4
Failed: 0
Workers: 4
Total Material Rows: 44
Total Cut Pipe Rows: 12
Wall Clock Time: 0.410 seconds
Total Processing Time: 1.530 seconds
Parallel Efficiency: 93.2%
Average Time per File: 0.383 seconds
============================================================
```

### Migration Benefits

Compared to the original Python implementation:

- **3-4x faster processing** due to Go's performance and better concurrency
- **Identical output format** for seamless integration with existing workflows
- **Enhanced error handling** and validation with detailed logging
- **Better resource utilization** with efficient parallel processing
- **Comprehensive debugging** support for troubleshooting

### Table Detection Logic

The extractor automatically identifies:

- **ERECTION MATERIALS tables** by title text and coordinate positioning
- **CUT PIPE LENGTH tables** with proper header alignment and data extraction
- **Drawing numbers** from title blocks and annotations (KKS codes)
- **Pipe classes** from center area annotations
- **Component categories** (PIPE, FITTINGS, VALVES, SUPPORTS, etc.)

## Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## Documentation

- **[README.md](README.md)** - Main user documentation and usage guide
- **[TECHNICAL_DOCUMENTATION.md](TECHNICAL_DOCUMENTATION.md)** - Comprehensive technical documentation for developers
- **[QUICK_REFERENCE.md](QUICK_REFERENCE.md)** - Quick reference for common commands and use cases
- **[MULTI_TABLE_DETECTION.md](MULTI_TABLE_DETECTION.md)** - Multi-table processing documentation

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Acknowledgments

- Built for efficient processing of technical CAD drawings
- Optimized for piping and instrumentation diagrams (P&IDs)
- Designed to handle large-scale engineering documentation