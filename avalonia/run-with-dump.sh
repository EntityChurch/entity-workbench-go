#!/usr/bin/env bash
# Launches entity-avalonia with full .NET crash diagnostics enabled.
# Lives in avalonia/ and is copied into dist-native by make up. Using
# a wrapper script instead of inline make env vars because make's
# line-continuation handling of env-prefixes was leaving the variables
# unexported in some shells, which silently disabled dump capture.

# These env vars MUST be exported (not just set) so .NET's startup
# native code sees them — the runtime reads them before any managed
# code runs.
export DOTNET_EnableDiagnostics=1
export DOTNET_DbgEnableMiniDump=1
export DOTNET_DbgMiniDumpType=4
export DOTNET_DbgMiniDumpName="$(pwd)/managed.%d.dmp"
export DOTNET_CreateDumpDiagnostics=1

# Avalonia's LogToTrace() writes to System.Diagnostics.Trace, which
# is silent unless a listener exports to stderr. The DOTNET_LogToConsole
# var ensures core runtime messages also reach stderr.
export DOTNET_LogToConsole=1

# Enable our breadcrumb log so each render / swap / load prints
# to stderr with a timestamp. The LAST line in run.log before the
# crash tells us what was happening at the moment of failure.
export WB_PANEL_LOG=1

export LD_LIBRARY_PATH=.

echo "==> entity-avalonia env:"
echo "    DOTNET_DbgMiniDumpName=$DOTNET_DbgMiniDumpName"
echo "    DOTNET_CreateDumpDiagnostics=$DOTNET_CreateDumpDiagnostics"
echo "    PWD=$(pwd)"
echo ""

exec ./entity-avalonia "$@"
