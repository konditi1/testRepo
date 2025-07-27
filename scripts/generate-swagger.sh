#!/bin/bash
set -e

echo "🔍 Checking if swag is installed..."
if ! command -v swag &> /dev/null; then
    echo "🔄 Installing swag..."
    go install github.com/swaggo/swag/cmd/swag@latest
fi

echo "📝 Generating Swagger documentation..."
# Fix: Point to the correct main.go file location and set proper search directory
swag init -g cmd/server/main.go -d ./ -o ./docs --parseDependency --parseInternal --parseVendor

if [ $? -eq 0 ]; then
    echo "✅ Swagger documentation generated successfully!"
    echo "📄 Documentation available at: docs/swagger.json"
    echo "🌐 You can view the Swagger UI at: http://localhost:9000/swagger/index.html"
    echo ""
    echo "📋 Generated files:"
    ls -la docs/
else
    echo "❌ Failed to generate Swagger documentation"
    exit 1
fi