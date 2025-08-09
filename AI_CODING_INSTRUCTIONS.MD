# DXF Text Parser Development Prompt
## ObjectiveCreate a high-performance Go program that extracts text content and coordinates from DXF files. The parser should be optimized for speed and handle 12MB+ files efficiently.
## Requirements
### Core Functionality- Extract all text entities (TEXT and MTEXT) from DXF files- Capture text content and X,Y coordinates for each entity- Return structured data that enables spatial analysis- Process files up to 12MB in under 2 seconds- Utilize 8 CPU cores through Go's concurrency features
### Technical Specifications
#### Input Processing- Read DXF files in standard ASCII format- Handle both TEXT and MTEXT entity types- Parse DXF group codes relevant to text extraction:  - Group code 0: Entity type identifier  - Group code 1: Primary text content  - Group code 3: Additional text content (for MTEXT)  - Group code 10: X coordinate  - Group code 20: Y coordinate  - Group code 40: Text height (optional for sizing context)
#### Output StructureCreate a struct that includes:- Text content (combined from multiple group code 1 and 3 entries if needed)- X coordinate (float64)- Y coordinate (float64)- Text height (optional)- Entity type (TEXT vs MTEXT)
#### Performance Requirements- Use goroutines to process file sections concurrently- Implement efficient memory usage (avoid loading entire file into memory)- Stream processing where possible- Target sub-second processing for 12MB files
### Code Structure
#### Main Components Needed1. **File Reader**: Concurrent file processing2. **DXF Parser**: Extract relevant group codes3. **Text Extractor**: Combine text fragments and coordinates4. **Spatial Analyzer**: Enable position-based queries5. **Test Suite**: Validate against provided sample files
#### Concurrent Architecture- Split file processing across available CPU cores- Use channels for inter-goroutine communication- Implement proper error handling and resource cleanup- Ensure thread-safe result aggregation
### Additional Features
#### Spatial Analysis Functions- Find text entities within specified coordinate ranges- Calculate relative positions between text elements- Support queries like "find all text in top-right quadrant relative to reference string"- Distance calculations between text entities
#### Error Handling- Graceful handling of malformed DXF files- Clear error messages for parsing failures- Validation of coordinate data- File access error management
### Testing Requirements
#### Test Coverage- Parse provided sample DXF files- Verify extracted text content accuracy- Validate coordinate precision- Measure parsing performance- Test concurrent processing under load
#### Benchmarking- Time parsing operations for different file sizes- Memory usage profiling- CPU utilization measurement- Comparison with single-threaded performance
### Code Quality Standards
#### Best Practices- Follow Go idioms and conventions- Include comprehensive error handling- Add meaningful comments for complex parsing logic- Use appropriate data structures for performance- Implement clean API design
#### Documentation- Function documentation with examples- Usage instructions- Performance characteristics explanation- Sample code for common use cases
## Deliverables
### Primary Code1. Complete Go parser with concurrent processing2. Spatial analysis helper functions3. Command-line interface for testing4. Benchmark suite for performance validation
### Supporting Materials1. Usage documentation with examples2. Performance analysis results3. Comparison with Python parsing speeds4. Recommendations for further optimization
## Testing Protocol
### File Processing Tests- Process all provided sample files- Extract and display first 10 text entities with coordinates- Verify no text entities are missed- Confirm coordinate accuracy against known positions
### Performance Validation- Measure parsing time for each sample file- Test with different numbers of goroutines (1, 2, 4, 8)- Profile memory usage during parsing- Generate performance comparison report
### Spatial Query Testing- Implement sample spatial queries using extracted data- Test relative positioning calculations- Validate "find text near reference" functionality
## Success Criteria
- Parser completes 12MB file processing in under 2 seconds- Extracts 100% of text entities without errors- Concurrent processing shows measurable speedup over single-threaded- Spatial queries return accurate position-based results- Code follows Go best practices and is maintainable
## Notes for Implementation
- DXF format uses paired lines (group code followed by value)- TEXT entities may span multiple lines for longer content- Coordinate system origin and scaling may vary between files- Consider using `bufio.Scanner` for efficient line-by-line processing- Implement graceful degradation if concurrent processing encounters issues
Please provide complete, production-ready code with proper error handling, documentation, and test examples. Include both single-threaded and concurrent versions for performance comparison.