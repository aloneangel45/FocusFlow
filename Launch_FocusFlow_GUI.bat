@echo off
setlocal
title FocusFlow v2.2.2 - Desktop HUD Launcher
cd /d "%~dp0"

where pythonw >nul 2>nul
if %ERRORLEVEL% EQU 0 (
    start "" pythonw "%~dp0FocusFlowGUI.py" %*
) else (
    start "" python "%~dp0FocusFlowGUI.py" %*
)
exit /b 0
