.PHONY: start stop build analyst

start:
	@echo "Starting Race Engineer..."
	./start.sh

stop:
	@echo "Stopping Race Engineer..."
	./stop.sh

build:
	@echo "Building Go telemetry-core..."
	cd telemetry-core && go build -o ../workspace/bin/telemetry-core cmd/server/main.go
	@echo "Building racedb query tool..."
	cd telemetry-core && go build -o ../workspace/bin/racedb cmd/query/main.go
	@echo "Building insightlog tool..."
	cd telemetry-core && go build -o ../workspace/bin/insightlog cmd/insightlog/main.go
	@echo "Build complete!"

analyst:
	@echo "Starting OpenCode analyst agent on port $${OPENCODE_PORT:-4095}..."
	cd workspace && opencode serve --port $${OPENCODE_PORT:-4095}
