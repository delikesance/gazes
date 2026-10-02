{ pkgs, lib, config, inputs, ... }:

{
  # Environment Variables
  env = {
    APP_ENV = "development";
    # Torrent piece completion uses BoltDB; diagnostics own the SQLite runtime.
    GOFLAGS = "-tags=nosqlite";
    PORT = "8090";
  };

  # Core Packages
  packages = [
    pkgs.git
    pkgs.curl
    pkgs.pkg-config
    pkgs.ffmpeg-full  # Includes ffmpeg, ffprobe, and extensive codec support
    pkgs.air          # Go live reload
    pkgs.golangci-lint
  ];

  # Language Toolchains
  languages.go = {
    enable = true;
  };

  languages.javascript = {
    enable = true;
    pnpm.enable = true;
  };

  languages.typescript = {
    enable = true;
  };

  # Start the complete detached stack and wait until all services are healthy.
  processes.stack.exec = ''
    cd ${lib.escapeShellArg config.devenv.root}
    exec docker compose up -d --build --wait
  '';

  # Development Helper Scripts
  scripts = {
    build-all.exec = ''
      echo "🔨 Building backend and frontend..."
      go build -v -o bin/gazes-server ./cmd/server
      pnpm --prefix web build
    '';
    test-all.exec = ''
      echo "🧪 Running backend test suites..."
      go test -v ./...
      echo "✨ Running frontend type check..."
      pnpm --prefix web build
    '';
  };

  enterShell = ''
    echo "🎬 Welcome to the Gazes Development Environment!"
    echo "  - Go: $(go version)"
    echo "  - Node: $(node --version)"
    echo "  - pnpm: $(pnpm --version 2>/dev/null || echo 'available')"
    echo "  - FFmpeg: $(ffmpeg -version | head -n 1)"
    echo "  - Commands: devenv up (builds & starts the Docker stack), build-all, test-all"
  '';
}
