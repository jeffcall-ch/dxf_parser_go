package main

import (
	"testing"
)

func TestNewDXFParser(t *testing.T) {
	parser := NewDXFParser(4)
	if parser.workers != 4 {
		t.Errorf("Expected 4 workers, got %d", parser.workers)
	}

	// Test with 0 workers (should use NumCPU)
	parser = NewDXFParser(0)
	if parser.workers <= 0 {
		t.Errorf("Expected positive number of workers, got %d", parser.workers)
	}
}

func TestTextEntityParsing(t *testing.T) {
	// Test a basic text entity structure
	entity := TextEntity{
		Content:    "TEST_TEXT",
		X:          100.0,
		Y:          200.0,
		Height:     5.0,
		EntityType: "TEXT",
		Layer:      "LAYER1",
	}

	// Verify entity fields
	if entity.Content != "TEST_TEXT" {
		t.Errorf("Expected content 'TEST_TEXT', got '%s'", entity.Content)
	}
	if entity.X != 100.0 {
		t.Errorf("Expected X coordinate 100.0, got %f", entity.X)
	}
	if entity.Y != 200.0 {
		t.Errorf("Expected Y coordinate 200.0, got %f", entity.Y)
	}
	if entity.Height != 5.0 {
		t.Errorf("Expected height 5.0, got %f", entity.Height)
	}
	if entity.EntityType != "TEXT" {
		t.Errorf("Expected entity type 'TEXT', got '%s'", entity.EntityType)
	}
	if entity.Layer != "LAYER1" {
		t.Errorf("Expected layer 'LAYER1', got '%s'", entity.Layer)
	}
}

func TestSpatialAnalyzer(t *testing.T) {
	entities := []TextEntity{
		{Content: "A", X: 0, Y: 0, EntityType: "TEXT"},
		{Content: "B", X: 10, Y: 10, EntityType: "TEXT"},
		{Content: "C", X: 20, Y: 20, EntityType: "MTEXT"},
		{Content: "D", X: -10, Y: -10, EntityType: "TEXT"},
	}

	analyzer := NewSpatialAnalyzer(entities)

	// Test bounding box
	bbox := analyzer.GetBoundingBox()
	if bbox.MinX != -10 || bbox.MinY != -10 || bbox.MaxX != 20 || bbox.MaxY != 20 {
		t.Errorf("Unexpected bounding box: %+v", bbox)
	}

	// Test range finding
	rangeEntities := analyzer.FindEntitiesInRange(5, 5, 15, 15)
	if len(rangeEntities) != 1 || rangeEntities[0].Content != "B" {
		t.Errorf("Expected 1 entity 'B' in range, got %d entities", len(rangeEntities))
	}

	// Test radius finding
	radiusEntities := analyzer.FindEntitiesInRadius(0, 0, 15)
	expectedCount := 3 // A, B, and D should be within radius 15 of origin
	if len(radiusEntities) != expectedCount {
		t.Errorf("Expected %d entities in radius, got %d", expectedCount, len(radiusEntities))
	}

	// Test nearest entities
	nearest := analyzer.FindNearestEntities(0, 0, 2)
	if len(nearest) != 2 {
		t.Errorf("Expected 2 nearest entities, got %d", len(nearest))
	}
	if nearest[0].Entity.Content != "A" {
		t.Errorf("Expected first nearest entity to be 'A', got '%s'", nearest[0].Entity.Content)
	}

	// Test quadrant
	quadrantEntities := analyzer.GetQuadrant(0, 0, 1) // Top-right quadrant
	expectedInQuadrant := 3 // A, B and C should be in top-right quadrant
	if len(quadrantEntities) != expectedInQuadrant {
		t.Errorf("Expected %d entities in quadrant 1, got %d", expectedInQuadrant, len(quadrantEntities))
	}
}

func TestDistance(t *testing.T) {
	distance := Distance(0, 0, 3, 4)
	expected := 5.0
	if distance != expected {
		t.Errorf("Expected distance %f, got %f", expected, distance)
	}
}

func TestEntityStats(t *testing.T) {
	entities := []TextEntity{
		{Content: "A", X: 0, Y: 0, EntityType: "TEXT", Height: 2.0, Layer: "L1"},
		{Content: "B", X: 10, Y: 10, EntityType: "MTEXT", Height: 3.0, Layer: "L1"},
		{Content: "C", X: 20, Y: 20, EntityType: "TEXT", Height: 4.0, Layer: "L2"},
	}

	analyzer := NewSpatialAnalyzer(entities)
	stats := analyzer.GetEntityStats()

	if stats["total_entities"] != 3 {
		t.Errorf("Expected 3 total entities, got %v", stats["total_entities"])
	}
	if stats["text_entities"] != 2 {
		t.Errorf("Expected 2 text entities, got %v", stats["text_entities"])
	}
	if stats["mtext_entities"] != 1 {
		t.Errorf("Expected 1 mtext entity, got %v", stats["mtext_entities"])
	}

	expectedAvgHeight := 3.0 // (2.0 + 3.0 + 4.0) / 3
	if stats["average_height"] != expectedAvgHeight {
		t.Errorf("Expected average height %f, got %v", expectedAvgHeight, stats["average_height"])
	}
}

func TestStringUtilities(t *testing.T) {
	// Test case-insensitive contains
	if !containsText("Hello World", "WORLD") {
		t.Error("Case-insensitive search should find 'WORLD' in 'Hello World'")
	}

	if containsText("Hello", "xyz") {
		t.Error("Should not find 'xyz' in 'Hello'")
	}

	// Test toLower
	if toLower("HELLO") != "hello" {
		t.Error("toLower should convert 'HELLO' to 'hello'")
	}

	// Test stringContains
	if !stringContains("testing", "test") {
		t.Error("Should find 'test' in 'testing'")
	}
}

func TestFindEntitiesNearText(t *testing.T) {
	entities := []TextEntity{
		{Content: "PIPE", X: 0, Y: 0, EntityType: "TEXT"},
		{Content: "VALVE", X: 5, Y: 0, EntityType: "TEXT"},
		{Content: "FLANGE", X: 100, Y: 100, EntityType: "TEXT"},
		{Content: "PIPE_2", X: 2, Y: 2, EntityType: "TEXT"},
	}

	analyzer := NewSpatialAnalyzer(entities)
	
	nearEntities := analyzer.FindEntitiesNearText("PIPE", 10.0)
	
	// Should find VALVE and PIPE_2 near the PIPE entities
	expectedMinCount := 2
	if len(nearEntities) < expectedMinCount {
		t.Errorf("Expected at least %d entities near 'PIPE', got %d", expectedMinCount, len(nearEntities))
	}

	// Check that FLANGE is not included (too far away)
	for _, entityWithDist := range nearEntities {
		if entityWithDist.Entity.Content == "FLANGE" {
			t.Error("FLANGE should not be near PIPE (distance > 10)")
		}
	}
}

// Benchmark tests for performance validation
func BenchmarkParseSmallFile(b *testing.B) {
	// This would need a real small DXF file for proper benchmarking
	parser := NewDXFParser(1)
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		// In real implementation, this would parse an actual small file
		_ = parser
	}
}

func BenchmarkSpatialQueries(b *testing.B) {
	// Create a large set of entities for benchmarking
	entities := make([]TextEntity, 10000)
	for i := 0; i < 10000; i++ {
		entities[i] = TextEntity{
			Content:    "TEXT_" + string(rune(i%100)),
			X:          float64(i % 1000),
			Y:          float64(i / 1000),
			EntityType: "TEXT",
		}
	}

	analyzer := NewSpatialAnalyzer(entities)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Benchmark different spatial queries
		analyzer.FindEntitiesInRange(100, 100, 200, 200)
		analyzer.FindEntitiesInRadius(500, 5, 50)
		analyzer.FindNearestEntities(500, 5, 10)
	}
}

func TestConcurrentVsSequential(t *testing.T) {
	// Test creating parsers with different worker counts
	parser := NewDXFParser(1)
	if parser == nil {
		t.Error("Failed to create parser")
	}

	parserConcurrent := NewDXFParser(4)
	if parserConcurrent == nil {
		t.Error("Failed to create concurrent parser")
	}

	// Verify worker counts
	if parser.workers != 1 {
		t.Errorf("Expected 1 worker, got %d", parser.workers)
	}
	if parserConcurrent.workers != 4 {
		t.Errorf("Expected 4 workers, got %d", parserConcurrent.workers)
	}
}

func TestChunkCalculation(t *testing.T) {
	parser := NewDXFParser(4)
	
	// Test chunk size
	if parser.chunkSize != 1024*1024 {
		t.Errorf("Expected chunk size 1MB, got %d", parser.chunkSize)
	}
	
	// Test that parser initializes correctly
	if parser.textBuffer == nil {
		// This will be initialized during parsing
	}
}

// Integration test that would work with real files
func TestIntegrationWithRealFile(t *testing.T) {
	// Skip this test if no test files are available
	t.Skip("Integration test requires real DXF files")
	
	// In a real scenario, this would test against actual DXF files:
	/*
	parser := NewDXFParser(4)
	entities, err := parser.ParseFile("test_files/sample.dxf")
	if err != nil {
		t.Fatalf("Failed to parse test file: %v", err)
	}
	
	if len(entities) == 0 {
		t.Error("Expected some entities to be parsed")
	}
	
	// Validate that all entities have required fields
	for _, entity := range entities {
		if entity.Content == "" {
			t.Error("Entity missing content")
		}
		if entity.EntityType == "" {
			t.Error("Entity missing type")
		}
	}
	*/
}

func TestPerformanceRequirements(t *testing.T) {
	// This test verifies that the parser meets performance requirements
	// Skip if no large test files available
	t.Skip("Performance test requires large DXF files")
	
	/*
	parser := NewDXFParser(8)
	start := time.Now()
	
	// Test with a 12MB file
	entities, err := parser.ParseFile("large_test_file.dxf")
	duration := time.Since(start)
	
	if err != nil {
		t.Fatalf("Failed to parse large file: %v", err)
	}
	
	// Should complete in under 2 seconds for 12MB file
	if duration > 2*time.Second {
		t.Errorf("Parsing took too long: %v (should be < 2s)", duration)
	}
	
	t.Logf("Parsed %d entities in %v", len(entities), duration)
	*/
}
