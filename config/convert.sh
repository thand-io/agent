#!/bin/bash

# Script to merge all YAML files (except all.yaml) into all.yaml and convert to JSON
# for each child folder

set -e

# Function to process a directory
process_directory() {
    local dir="$1"
    echo "Processing directory: $dir"
    
    if [ ! -d "$dir" ]; then
        echo "Directory $dir does not exist, skipping..."
        return
    fi
    
    cd "$dir"
    
    # Find all .yaml files except all.yaml
    yaml_files=($(find . -maxdepth 1 -name "*.yaml" ! -name "all.yaml" | sort))
    
    if [ ${#yaml_files[@]} -eq 0 ]; then
        echo "No YAML files found in $dir (excluding all.yaml)"
        cd - > /dev/null
        return
    fi
    
    echo "Found YAML files: ${yaml_files[*]}"
    
    # Merge all YAML files into all.yaml using yq
    # Start with an empty YAML document
    echo "{}" | yq -P > all.yaml
    
    # Merge each YAML file using yq to properly combine structures
    for yaml_file in "${yaml_files[@]}"; do
        if [ -f "$yaml_file" ]; then
            echo "Merging $yaml_file..."
            # Use yq to merge the YAML files, combining matching root keys
            yq eval-all '. as $item ireduce ({}; . * $item)' all.yaml "$yaml_file" > temp_merged.yaml
            mv temp_merged.yaml all.yaml
        fi
    done
    
    # Convert all.yaml to all.json
    echo "Converting all.yaml to all.json..."
    yq --prettyPrint -o=json all.yaml > all.json
    
    echo "Completed processing $dir"
    echo ""
    
    cd - > /dev/null
}

# Get the script's directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Process each child directory
for dir in "$SCRIPT_DIR"/*/; do
    if [ -d "$dir" ]; then
        process_directory "$dir"
    fi
done

echo "All directories processed successfully!" 

