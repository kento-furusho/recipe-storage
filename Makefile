build-RecipeAPIFunction:
	@echo "Building Lambda function..."
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build \
		-tags lambda,lambda.noroot \
		-o $(ARTIFACTS_DIR)/bootstrap \
		-ldflags="-s -w" \
		.
	@echo "Build complete!"

build-RecipeWorkerFunction:
	@echo "Building worker Lambda function..."
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build \
		-tags worker,lambda.noroot \
		-o $(ARTIFACTS_DIR)/bootstrap \
		-ldflags="-s -w" \
		.
	@echo "Build complete!"
