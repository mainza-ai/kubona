#!/bin/sh
# Regenerate protobuf bindings from the single source of truth:
#   platform/bridge/proto/kubona.proto  (root proto/kubona.proto is a synced mirror)
#
# Pinned toolchain (must match the version stamps in the generated files):
#   protoc            v7.35.1  (system, `protoc --version`)
#   protoc-gen-go     v1.36.12 (GOBIN install: go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.12)
#   protoc-gen-go-grpc v1.6.2  (GOBIN install: go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.6.2)
#   grpcio-tools      1.71.x   (scratch venv only — NEVER the hermetic platform/.venv;
#                               1.71 stamps "Protobuf Python Version: 5.29.0" matching the
#                               runtime. Newer grpcio-tools emits 7.x gencode that the
#                               5.29.6 runtime refuses to import.)
#
# Usage: ./scripts/regen-proto.sh   (repo root; writes all four bindings in place)
# After running: `git diff --stat` must show ONLY the intended message change.
# A noisy diff means toolchain drift — stop, do not commit.
set -eu
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SRC="$ROOT/platform/bridge/proto/kubona.proto"
GOBIN_DIR="${GOBIN:-$HOME/go/bin}"
export PATH="$GOBIN_DIR:$PATH"

protoc -I"$ROOT/platform/bridge/proto" \
  --plugin=protoc-gen-go="$GOBIN_DIR/protoc-gen-go" \
  --go_out="$ROOT/platform/core/proto" --go_opt=paths=source_relative \
  --plugin=protoc-gen-go-grpc="$GOBIN_DIR/protoc-gen-go-grpc" \
  --go-grpc_out="$ROOT/platform/core/proto" --go-grpc_opt=paths=source_relative \
  "$SRC"

SCRATCH="${PROTOGEN_VENV:-/tmp/protogen}"
"$SCRATCH/bin/python" -m grpc_tools.protoc -I"$ROOT/platform/bridge/proto" \
  --python_out="$ROOT/platform/bridge/proto" \
  --grpc_python_out="$ROOT/platform/bridge/proto" \
  "$SRC"

cp "$SRC" "$ROOT/proto/kubona.proto"
echo "regenerated: core/proto/*.pb.go, bridge/proto/kubona_pb2*.py, proto/kubona.proto mirror"
echo "verify: git diff --stat (only the intended change may appear)"
