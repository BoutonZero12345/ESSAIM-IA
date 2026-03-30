# ESSAIM IA — Journal Generator
# Formats essaim.log into a beautiful human-readable journal
# Usage: powershell ./tools/journal.ps1

$logPath = "$PSScriptRoot\..\logs\essaim.log"
$journalPath = "$PSScriptRoot\..\logs\journal.md"

if (-not (Test-Path $logPath)) {
    Write-Host "❌ Fichier log introuvable: $logPath" -ForegroundColor Red
    exit 1
}

$lines = Get-Content $logPath -Encoding UTF8
$journal = @()
$journal += "# 📋 ESSAIM IA — Journal de Mission"
$journal += ""
$journal += "> Généré le $(Get-Date -Format 'yyyy-MM-dd HH:mm:ss')"
$journal += ""
$journal += "---"
$journal += ""

$currentSection = ""
$rawOutputBuffer = @()
$inRawBlock = $false

foreach ($line in $lines) {
    # Skip empty lines in non-raw mode
    if (-not $inRawBlock -and [string]::IsNullOrWhiteSpace($line)) { continue }

    # Detect JSON blocks (raw LLM output)
    if ($line -match '^\{' -or $line -match '^\[') {
        $inRawBlock = $true
        $rawOutputBuffer += $line
        continue
    }
    if ($inRawBlock) {
        $rawOutputBuffer += $line
        if ($line -match '^\}' -or $line -match '^\]') {
            $journal += '```json'
            $journal += $rawOutputBuffer
            $journal += '```'
            $journal += ""
            $rawOutputBuffer = @()
            $inRawBlock = $false
        }
        continue
    }

    # Parse log line
    if ($line -match '^\d{4}/\d{2}/\d{2} (\d{2}:\d{2}:\d{2}) \S+: (.+)$') {
        $time = $Matches[1]
        $content = $Matches[2]

        # Section headers
        if ($content -match '={10,}') {
            $journal += "---"
            continue
        }

        # Server startup
        if ($content -match 'ESSAIM IA.*Backend') {
            $section = "🚀 Démarrage du Serveur"
            if ($currentSection -ne $section) {
                $journal += "## $section"
                $journal += ""
                $currentSection = $section
            }
            $journal += "**$time** — $content"
            continue
        }

        # Config
        if ($content -match '\[CONFIG\]') {
            $section = "⚙️ Configuration"
            if ($currentSection -ne $section) {
                $journal += ""
                $journal += "## $section"
                $journal += ""
                $currentSection = $section
            }
            $clean = $content -replace '\[CONFIG\]\s*', ''
            $journal += "- ``$time`` $clean"
            continue
        }

        # OK status
        if ($content -match '\[OK\]') {
            $section = "✅ Initialisation"
            if ($currentSection -ne $section) {
                $journal += ""
                $journal += "## $section"
                $journal += ""
                $currentSection = $section
            }
            $clean = $content -replace '\[OK\]\s*', ''
            $journal += "- ``$time`` $clean"
            continue
        }

        # Genesis
        if ($content -match 'GENESIS') {
            $section = "🌟 Genèse — Création de l'Agent Alpha"
            if ($currentSection -ne $section) {
                $journal += ""
                $journal += "## $section"
                $journal += ""
                $currentSection = $section
            }
            $journal += "**$time** — $content"
            $journal += ""
            continue
        }

        # Processor events
        if ($content -match '\[PROCESSOR\]') {
            $section = "🧠 Traitement des Agents"
            if ($currentSection -ne $section) {
                $journal += ""
                $journal += "## $section"
                $journal += ""
                $currentSection = $section
            }
            $clean = $content -replace '\[PROCESSOR\]\s*', ''
            $journal += "- ``$time`` $clean"
            continue
        }

        # Dispatcher
        if ($content -match '\[DISPATCHER\]') {
            $clean = $content -replace '\[DISPATCHER\]\s*', ''
            $journal += "- ``$time`` 🔧 DISPATCHER: $clean"
            continue
        }

        # Grim Reaper
        if ($content -match 'GRIM REAPER') {
            $journal += "- ``$time`` 💀 $content"
            continue
        }

        # WebSocket
        if ($content -match '\[WS') {
            $clean = $content -replace '\[WS.*?\]\s*', ''
            $journal += "- ``$time`` 🔌 $clean"
            continue
        }

        # API calls
        if ($content -match '\[API\]') {
            $section = "📡 Appels API"
            if ($currentSection -ne $section) {
                $journal += ""
                $journal += "## $section"
                $journal += ""
                $currentSection = $section
            }
            $clean = $content -replace '\[API\]\s*', ''
            $journal += "- ``$time`` $clean"
            continue
        }

        # Server events
        if ($content -match '\[SERVER\]') {
            $clean = $content -replace '\[SERVER\]\s*', ''
            $journal += "- ``$time`` 🖥️ $clean"
            continue
        }

        # Fallback
        $journal += "- ``$time`` $content"
    } else {
        # Non-parseable lines (continuation)
        $journal += "  $line"
    }
}

# Summary section
$journal += ""
$journal += "---"
$journal += ""
$journal += "## 📊 Résumé"
$journal += ""

$agentCount = ($lines | Where-Object { $_ -match 'Spawned child|Agent Alpha' }).Count
$llmCalls = ($lines | Where-Object { $_ -match 'Calling LLM' }).Count
$errors = ($lines | Where-Object { $_ -match '❌' }).Count
$panics = ($lines | Where-Object { $_ -match 'PANIC' }).Count

$journal += "| Métrique | Valeur |"
$journal += "|---|---|"
$journal += "| Agents créés | $agentCount |"
$journal += "| Appels LLM | $llmCalls |"
$journal += "| Erreurs | $errors |"
$journal += "| Panics récupérés | $panics |"

$journal | Out-File -FilePath $journalPath -Encoding UTF8
Write-Host "✅ Journal généré: $journalPath" -ForegroundColor Green
Write-Host "   $agentCount agents, $llmCalls appels LLM, $errors erreurs" -ForegroundColor Cyan
