/*
Copyright 2019 The Cloud-Barista Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package common is to include common methods for managing multi-cloud infra
package common

import (
	"fmt"
	"regexp"
	"strings"

)

func ExtractOSInfo(combinedInfo string) string {
	if combinedInfo == "" {
		return ""
	}

	infoLower := strings.ToLower(combinedInfo)

	// Loop through all OS types from extraction patterns
	for _, osInfo := range RuntimeExtractPatternsInfo.ExtractPatterns.OSType {
		// Check if OS patterns are in the combined string
		for _, pattern := range osInfo.Patterns {
			if strings.Contains(infoLower, strings.ToLower(pattern)) {
				// OS found, now look for version
				for _, version := range osInfo.Versions {
					// Take only the numeric parts of the version
					numericParts := regexp.MustCompile(`\d+`).FindAllString(version, -1)
					if len(numericParts) > 0 {
						// Join the numeric parts with a regex pattern to match any non-numeric characters
						// This excludes date formats like 22T04
						versionPattern := strings.Join(numericParts, "[.\\s-_]?")
						// Add a regex pattern to avoid matching the version in the middle of numeric strings
						boundedVersionPattern := fmt.Sprintf("(?:^|[^0-9])%s(?:$|[^0-9])", versionPattern)

						re := regexp.MustCompile(boundedVersionPattern)
						if re.MatchString(infoLower) {
							return fmt.Sprintf("%s %s", osInfo.Name, version)
						}
					}
				}

				// OS found but no specific version, return the OS name without version
				return osInfo.Name
			}
		}
	}

	// If no match found, return empty string
	return ""
}

// CheckBasicOSImage checks if the given combined info matches any basic OS image patterns
// Returns true if the image is considered a basic OS image based on predefined patterns
// providerName is used to apply CSP-specific rules (if defined) or common rules (as fallback)
//
// Logic:
// 1. Determine include patterns:
//   - If CSP-specific include exists and not empty → use CSP-specific include
//   - Otherwise → use common include
//
// 2. Determine exclude patterns:
//   - If CSP-specific exclude exists and not empty → use CSP-specific exclude
//   - Otherwise → use common exclude
//
// 3. Check exclude patterns first (return false if matched)
// 4. Check include patterns (return true if matched)
func CheckBasicOSImage(combinedInfo string, providerName string) bool {
	if combinedInfo == "" {
		return false
	}

	// Normalize provider name to lowercase for consistent matching
	providerNameLower := strings.ToLower(providerName)

	// Loop through all OS types from extraction patterns
	for _, osInfo := range RuntimeExtractPatternsInfo.ExtractPatterns.OSType {
		// Skip if no basic image rules defined
		if osInfo.BasicImageRules == nil {
			continue
		}

		// Start with common patterns as default
		includePatterns := osInfo.BasicImageRules.Common.Include
		excludePatterns := osInfo.BasicImageRules.Common.Exclude

		// Override with CSP-specific patterns if they exist and are not empty
		if osInfo.BasicImageRules.CspSpecific != nil {
			if cspPatterns, exists := osInfo.BasicImageRules.CspSpecific[providerNameLower]; exists {
				// Use CSP-specific include if defined and not empty
				if len(cspPatterns.Include) > 0 {
					includePatterns = cspPatterns.Include
				}
				// Use CSP-specific exclude if defined and not empty
				if len(cspPatterns.Exclude) > 0 {
					excludePatterns = cspPatterns.Exclude
				}
			}
		}

		// Step 1: Check exclude patterns first (higher priority)
		for _, excludePattern := range excludePatterns {
			if matchesPattern(combinedInfo, excludePattern) {
				// This image matches an exclude pattern, so it's not a basic image
				return false
			}
		}

		// Step 2: Check include patterns
		for _, includePattern := range includePatterns {
			if matchesPattern(combinedInfo, includePattern) {
				// Pattern matched and not excluded
				return true
			}
		}
	}

	return false
}

// matchesPattern checks if a string matches a pattern with wildcard support
// The pattern supports '*' as wildcard that matches any sequence of characters
func matchesPattern(text, pattern string) bool {
	if pattern == "" {
		return text == ""
	}

	// Split pattern by '*' to get literal parts
	parts := strings.Split(pattern, "*")

	// If no wildcards, do exact match (case-insensitive)
	if len(parts) == 1 {
		return strings.EqualFold(text, pattern)
	}

	textLower := strings.ToLower(text)

	// Check if text starts with first part
	if len(parts[0]) > 0 {
		if !strings.HasPrefix(textLower, strings.ToLower(parts[0])) {
			return false
		}
		textLower = textLower[len(parts[0]):]
	}

	// Check if text ends with last part
	if len(parts[len(parts)-1]) > 0 {
		lastPart := strings.ToLower(parts[len(parts)-1])
		if !strings.HasSuffix(textLower, lastPart) {
			return false
		}
		textLower = textLower[:len(textLower)-len(lastPart)]
	}

	// Check middle parts in order
	for i := 1; i < len(parts)-1; i++ {
		part := strings.ToLower(parts[i])
		if part == "" {
			continue // empty part between consecutive wildcards
		}

		index := strings.Index(textLower, part)
		if index == -1 {
			return false
		}

		// Move past this part
		textLower = textLower[index+len(part):]
	}

	return true
}

// IsGPUImage checks if an image has GPU support
func IsGPUImage(combinedInfo string) bool {
	if combinedInfo == "" {
		return false
	}

	infoLower := strings.ToLower(combinedInfo)

	// Check exclude patterns first — any match overrides positive GPU patterns
	for _, exc := range RuntimeExtractPatternsInfo.ExtractPatterns.GPUExcludePatterns {
		if strings.Contains(infoLower, strings.ToLower(exc)) {
			return false
		}
	}

	for _, pattern := range RuntimeExtractPatternsInfo.ExtractPatterns.GPUPatterns {
		if strings.Contains(infoLower, strings.ToLower(pattern)) {
			return true
		}
	}

	return false
}

// CheckBasicGpuImage checks if the given combined info matches any basic GPU image patterns.
// Uses CSP-specific include patterns with a common exclude pre-filter.
func CheckBasicGpuImage(combinedInfo string, providerName string) bool {
	rules := RuntimeExtractPatternsInfo.ExtractPatterns.BasicGpuImageRules
	if rules == nil {
		return false
	}

	providerNameLower := strings.ToLower(providerName)

	// Apply common exclude patterns first
	for _, exc := range rules.Common.Exclude {
		if matchesPattern(combinedInfo, exc) {
			return false
		}
	}

	// Determine include patterns: CSP-specific overrides common
	includePatterns := rules.Common.Include
	if rules.CspSpecific != nil {
		if cspPatterns, exists := rules.CspSpecific[providerNameLower]; exists && len(cspPatterns.Include) > 0 {
			includePatterns = cspPatterns.Include
		}
	}

	for _, pattern := range includePatterns {
		if matchesPattern(combinedInfo, pattern) {
			return true
		}
	}

	return false
}

// IsK8sImage checks if an image is for Kubernetes
func IsK8sImage(combinedInfo string) bool {
	if combinedInfo == "" {
		return false
	}

	infoLower := strings.ToLower(combinedInfo)

	for _, pattern := range RuntimeExtractPatternsInfo.ExtractPatterns.K8sPatterns {
		if strings.Contains(infoLower, strings.ToLower(pattern)) {
			return true
		}
	}

	return false
}

// IsDeprecatedImage checks if an image is deprecated
func IsDeprecatedImage(combinedInfo string) bool {
	if combinedInfo == "" {
		return false
	}

	infoLower := strings.ToLower(combinedInfo)

	// Check for deprecated patterns
	patterns := []string{
		"deprecated",
		"end of life",
		"eol",
	}

	for _, pattern := range patterns {
		if strings.Contains(infoLower, pattern) {
			return true
		}
	}

	return false
}

// ConvertToBaseCurrency converts a cost value from a specific currency to the base currency (USD)
// This function handles currency conversion using predefined exchange rates
