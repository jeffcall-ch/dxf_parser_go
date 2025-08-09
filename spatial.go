package main

import (
	"math"
	"sort"
)

// SpatialAnalyzer provides spatial analysis functions for text entities
type SpatialAnalyzer struct {
	entities []TextEntity
}

// NewSpatialAnalyzer creates a new spatial analyzer with the given entities
func NewSpatialAnalyzer(entities []TextEntity) *SpatialAnalyzer {
	return &SpatialAnalyzer{entities: entities}
}

// Distance calculates the Euclidean distance between two points
func Distance(x1, y1, x2, y2 float64) float64 {
	dx := x2 - x1
	dy := y2 - y1
	return math.Sqrt(dx*dx + dy*dy)
}

// BoundingBox represents a rectangular boundary
type BoundingBox struct {
	MinX, MinY, MaxX, MaxY float64
}

// GetBoundingBox calculates the bounding box of all text entities
func (sa *SpatialAnalyzer) GetBoundingBox() BoundingBox {
	if len(sa.entities) == 0 {
		return BoundingBox{}
	}

	bbox := BoundingBox{
		MinX: sa.entities[0].X,
		MinY: sa.entities[0].Y,
		MaxX: sa.entities[0].X,
		MaxY: sa.entities[0].Y,
	}

	for _, entity := range sa.entities {
		if entity.X < bbox.MinX {
			bbox.MinX = entity.X
		}
		if entity.X > bbox.MaxX {
			bbox.MaxX = entity.X
		}
		if entity.Y < bbox.MinY {
			bbox.MinY = entity.Y
		}
		if entity.Y > bbox.MaxY {
			bbox.MaxY = entity.Y
		}
	}

	return bbox
}

// FindEntitiesInRange returns all text entities within the specified coordinate range
func (sa *SpatialAnalyzer) FindEntitiesInRange(minX, minY, maxX, maxY float64) []TextEntity {
	var result []TextEntity
	
	for _, entity := range sa.entities {
		if entity.X >= minX && entity.X <= maxX && entity.Y >= minY && entity.Y <= maxY {
			result = append(result, entity)
		}
	}
	
	return result
}

// FindEntitiesInRadius returns all text entities within the specified radius of a point
func (sa *SpatialAnalyzer) FindEntitiesInRadius(centerX, centerY, radius float64) []TextEntity {
	var result []TextEntity
	radiusSquared := radius * radius
	
	for _, entity := range sa.entities {
		dx := entity.X - centerX
		dy := entity.Y - centerY
		distanceSquared := dx*dx + dy*dy
		
		if distanceSquared <= radiusSquared {
			result = append(result, entity)
		}
	}
	
	return result
}

// EntityWithDistance represents an entity with its distance from a reference point
type EntityWithDistance struct {
	Entity   TextEntity
	Distance float64
}

// FindNearestEntities returns the N nearest entities to the specified point
func (sa *SpatialAnalyzer) FindNearestEntities(x, y float64, n int) []EntityWithDistance {
	if n <= 0 || len(sa.entities) == 0 {
		return nil
	}

	// Calculate distances for all entities
	entitiesWithDistance := make([]EntityWithDistance, len(sa.entities))
	for i, entity := range sa.entities {
		distance := Distance(x, y, entity.X, entity.Y)
		entitiesWithDistance[i] = EntityWithDistance{
			Entity:   entity,
			Distance: distance,
		}
	}

	// Sort by distance
	sort.Slice(entitiesWithDistance, func(i, j int) bool {
		return entitiesWithDistance[i].Distance < entitiesWithDistance[j].Distance
	})

	// Return the first N entities
	if n > len(entitiesWithDistance) {
		n = len(entitiesWithDistance)
	}
	
	return entitiesWithDistance[:n]
}

// FindEntitiesNearText finds all entities within a specified distance of entities containing the given text
func (sa *SpatialAnalyzer) FindEntitiesNearText(searchText string, maxDistance float64) []EntityWithDistance {
	var referenceEntities []TextEntity
	var result []EntityWithDistance
	
	// Find all entities containing the search text
	for _, entity := range sa.entities {
		if containsText(entity.Content, searchText) {
			referenceEntities = append(referenceEntities, entity)
		}
	}
	
	if len(referenceEntities) == 0 {
		return result
	}
	
	// Find entities near any of the reference entities
	seen := make(map[int]bool) // To avoid duplicates
	
	for _, refEntity := range referenceEntities {
		for i, entity := range sa.entities {
			if seen[i] {
				continue
			}
			
			distance := Distance(refEntity.X, refEntity.Y, entity.X, entity.Y)
			if distance <= maxDistance {
				result = append(result, EntityWithDistance{
					Entity:   entity,
					Distance: distance,
				})
				seen[i] = true
			}
		}
	}
	
	// Sort by distance
	sort.Slice(result, func(i, j int) bool {
		return result[i].Distance < result[j].Distance
	})
	
	return result
}

// GetQuadrant returns entities in a specific quadrant relative to a reference point
// quadrant: 1=top-right, 2=top-left, 3=bottom-left, 4=bottom-right
func (sa *SpatialAnalyzer) GetQuadrant(refX, refY float64, quadrant int) []TextEntity {
	var result []TextEntity
	
	for _, entity := range sa.entities {
		dx := entity.X - refX
		dy := entity.Y - refY
		
		switch quadrant {
		case 1: // Top-right
			if dx >= 0 && dy >= 0 {
				result = append(result, entity)
			}
		case 2: // Top-left
			if dx <= 0 && dy >= 0 {
				result = append(result, entity)
			}
		case 3: // Bottom-left
			if dx <= 0 && dy <= 0 {
				result = append(result, entity)
			}
		case 4: // Bottom-right
			if dx >= 0 && dy <= 0 {
				result = append(result, entity)
			}
		}
	}
	
	return result
}

// FindEntitiesInTopRightQuadrant finds entities in top-right quadrant relative to entities containing search text
func (sa *SpatialAnalyzer) FindEntitiesInTopRightQuadrant(searchText string) []TextEntity {
	var result []TextEntity
	seen := make(map[int]bool)
	
	// Find all entities containing the search text
	for _, refEntity := range sa.entities {
		if containsText(refEntity.Content, searchText) {
			quadrantEntities := sa.GetQuadrant(refEntity.X, refEntity.Y, 1)
			for _, entity := range quadrantEntities {
				// Use a simple hash to avoid duplicates
				hash := int(entity.X*1000 + entity.Y*1000)
				if !seen[hash] {
					result = append(result, entity)
					seen[hash] = true
				}
			}
		}
	}
	
	return result
}

// GetEntityStats returns statistical information about the entities
func (sa *SpatialAnalyzer) GetEntityStats() map[string]interface{} {
	if len(sa.entities) == 0 {
		return map[string]interface{}{
			"total_entities": 0,
		}
	}

	bbox := sa.GetBoundingBox()
	
	// Count by entity type
	textCount := 0
	mtextCount := 0
	totalHeight := 0.0
	heightCount := 0
	
	layerCounts := make(map[string]int)
	
	for _, entity := range sa.entities {
		if entity.EntityType == "TEXT" {
			textCount++
		} else if entity.EntityType == "MTEXT" {
			mtextCount++
		}
		
		if entity.Height > 0 {
			totalHeight += entity.Height
			heightCount++
		}
		
		if entity.Layer != "" {
			layerCounts[entity.Layer]++
		}
	}
	
	avgHeight := 0.0
	if heightCount > 0 {
		avgHeight = totalHeight / float64(heightCount)
	}
	
	return map[string]interface{}{
		"total_entities":     len(sa.entities),
		"text_entities":      textCount,
		"mtext_entities":     mtextCount,
		"bounding_box":       bbox,
		"average_height":     avgHeight,
		"layer_distribution": layerCounts,
		"drawing_width":      bbox.MaxX - bbox.MinX,
		"drawing_height":     bbox.MaxY - bbox.MinY,
	}
}

// containsText checks if the content contains the search text (case-insensitive)
func containsText(content, searchText string) bool {
	return len(content) > 0 && len(searchText) > 0 && 
		   stringContainsIgnoreCase(content, searchText)
}

// stringContainsIgnoreCase performs case-insensitive substring search
func stringContainsIgnoreCase(s, substr string) bool {
	s = toLower(s)
	substr = toLower(substr)
	return stringContains(s, substr)
}

// toLower converts string to lowercase (simple implementation)
func toLower(s string) string {
	result := make([]byte, len(s))
	for i, b := range []byte(s) {
		if b >= 'A' && b <= 'Z' {
			result[i] = b + 32
		} else {
			result[i] = b
		}
	}
	return string(result)
}

// stringContains checks if s contains substr
func stringContains(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	if len(substr) > len(s) {
		return false
	}
	
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
