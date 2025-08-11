@echo off
echo Building DXF Isometric Pipe Drawing Analyzer Tools...
echo.

echo [1/2] Building Unified BOM and Cut Length Extractor...
C:\Users\szil\Software\go\bin\go.exe build -o bom_cut_length_extractor.exe main.go cut_length_extractor.go cli.go spatial.go table_extraction.go bom_utils.go bom_extractor.go
if %errorlevel% neq 0 (
    echo ERROR: Failed to build bom_cut_length_extractor.exe
    exit /b 1
)

echo [2/2] Building Weld Symbol Detector...
C:\Users\szil\Software\go\bin\go.exe build -o weld_detector.exe weld_detector.go
if %errorlevel% neq 0 (
    echo ERROR: Failed to build weld_detector.exe
    exit /b 1
)

echo.
echo ✅ All tools built successfully!
echo.
echo Available executables:
echo   - bom_cut_length_extractor.exe  (Extract pipe components, cut lengths, and generate BOM)
echo   - weld_detector.exe             (Count weld symbols in isometric drawings)
echo.
echo Usage examples:
echo   bom_cut_length_extractor.exe bom -dir drawings_folder
echo   bom_cut_length_extractor.exe parse single_drawing.dxf
echo   weld_detector.exe -file drawing.dxf
echo   weld_detector.exe -dir drawings_folder
