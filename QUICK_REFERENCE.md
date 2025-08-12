# DXF Parser Go - Quick Reference

## Common Commands

### Standard BOM Extraction
```bash
# Basic usage - extracts BOM from all DXF files in directory
./bom_cut_length_extractor.exe bom -dir "path/to/drawings"

# With debug output for troubleshooting
./bom_cut_length_extractor.exe bom -dir "path/to/drawings" -debug

# Custom worker count (default is auto-detected)
./bom_cut_length_extractor.exe bom -dir "path/to/drawings" -workers 4
```

### Integrated Weld Analysis (RECOMMENDED)
```bash
# BOM extraction + enhanced weld detection
./bom_cut_length_extractor.exe bom -dir "path/to/drawings" -weld

# With detailed debugging
./bom_cut_length_extractor.exe bom -dir "path/to/drawings" -weld -debug
```

### Standalone Weld Detection
```bash
# Single file
./weld_detector.exe -file "drawing.dxf"

# Directory processing
./weld_detector.exe -dir "path/to/drawings"

# Custom output filename
./weld_detector.exe -dir "path/to/drawings" -output "my_weld_results.csv"
```

## Output Files

| File | Content | When Generated |
|------|---------|----------------|
| `0001_ERECTION_MATERIALS.csv` | Complete materials list with descriptions | Always |
| `0002_CUT_PIPE_LENGTH.csv` | Pipe cut lengths with piece numbers | Always |
| `0003_AGGREGATED_MATERIALS.csv` | Summarized materials by type | Always |
| `0004_SUMMARY.csv` | Processing summary and statistics | Always |
| `0005_WELD_COUNTS.csv` | Enhanced weld analysis with pipe info | Only with `-weld` flag |

## Key Features by Use Case

### Engineering BOM Generation
**Goal**: Extract materials list for procurement and planning
```bash
./bom_cut_length_extractor.exe bom -dir "iso_drawings" -debug
```
**Output**: Materials list with quantities, cut lengths, and processing summary

### Weld Planning & Estimation
**Goal**: Count welds and correlate with pipe information for welding planning
```bash
./bom_cut_length_extractor.exe bom -dir "iso_drawings" -weld
```
**Output**: Weld counts with pipe sizes, descriptions, and complexity indicators

### Quality Control & Validation
**Goal**: Verify drawing completeness and data quality
```bash
./bom_cut_length_extractor.exe bom -dir "drawings" -weld -debug -workers 1
```
**Output**: Detailed processing logs, error reports, and comprehensive data analysis

### Batch Processing for Projects
**Goal**: Process large numbers of drawings efficiently
```bash
./bom_cut_length_extractor.exe bom -dir "project_drawings" -weld -workers 8
```
**Output**: High-throughput processing with parallel worker optimization

## Performance Guidelines

### File Organization
- **Recommended**: Place all DXF files in a single directory for batch processing
- **Avoid**: Processing individual files separately (much slower)

### Worker Configuration
- **Small batches (1-10 files)**: Use default auto-detection
- **Large batches (50+ files)**: Consider `-workers 8` for maximum throughput
- **Memory constraints**: Use `-workers 4` to reduce memory usage

### Troubleshooting
```bash
# If processing fails, enable debug mode
./bom_cut_length_extractor.exe bom -dir "problem_files" -debug -workers 1

# Check specific file that's causing issues
./bom_cut_length_extractor.exe parse "problem_file.dxf"
```

## Output Format Reference

### Enhanced Weld CSV Columns
```
FilePath          - Full path to processed DXF file
FileName          - Base filename without extension  
DrawingNo         - Extracted drawing/KKS number
PipeClass         - Pipe classification from BOM
PipeNS            - Comma-separated pipe nominal sizes (sorted)
PipeDescription   - Full pipe descriptions from BOM
MultiplePipeNS    - "Yes" if multiple pipe sizes detected
WeldCount         - Number of weld symbols found
ProcessingTime    - Processing time in seconds
Error             - Any processing errors
```

### Common Pipe NS Examples
- `"15"` - Single 15mm pipe
- `"25, 40"` - Multiple sizes: 25mm and 40mm pipes
- `"15, 20, 25"` - Complex drawing with three different sizes

### Multiple Pipe Size Indicator
- **Empty**: Single pipe size throughout drawing
- **"Yes"**: Multiple pipe sizes detected (more complex welding requirements)

## Integration Examples

### Excel Integration
```bash
# Generate CSV files that can be directly imported into Excel
./bom_cut_length_extractor.exe bom -dir "drawings" -weld

# Files can be opened directly in Excel or imported into existing worksheets
```

### ERP System Integration
```bash
# Consistent CSV format for automated import into ERP systems
# Files follow standard naming convention for automated processing pipelines
```

### Project Management Integration
```bash
# Use 0004_SUMMARY.csv for project tracking
# Use 0005_WELD_COUNTS.csv for resource planning and scheduling
```

## Error Handling

### Common Issues
1. **"No DXF files found"** - Check directory path and file extensions
2. **"Permission denied"** - Ensure write access to output directory
3. **"Out of memory"** - Reduce worker count or process smaller batches

### Debug Mode Benefits
- Detailed file processing logs
- Entity count statistics  
- Table detection information
- Performance timing breakdown
- Error context and recovery attempts

## Version History

### Latest Features
- ✅ Integrated weld detection with BOM extraction
- ✅ Enhanced CSV output with pipe information correlation
- ✅ Performance caching for combined operations
- ✅ Multiple pipe size detection and flagging
- ✅ Improved error handling and isolation

### Performance Improvements
- **3-4x faster** than original Python implementation
- **2x faster** than separate BOM + weld tools
- **90%+ parallel efficiency** with optimal worker configuration
- **Memory optimized** for large file batches
