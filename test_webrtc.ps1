# WebRTC Transfer Test Script
# Tests the WebRTC P2P file transfer (default mode without --local flag)

Write-Host "=== WireGo WebRTC Transfer Test ===" -ForegroundColor Cyan
Write-Host ""

# Create test directory structure
$testDir = ".\test_webrtc"
$receiveDir = ".\test_webrtc_received"

# Clean up previous test
if (Test-Path $testDir) { Remove-Item -Recurse -Force $testDir }
if (Test-Path $receiveDir) { Remove-Item -Recurse -Force $receiveDir }

# Create test structure with different file types
New-Item -ItemType Directory -Path "$testDir\images" -Force | Out-Null
New-Item -ItemType Directory -Path "$testDir\videos" -Force | Out-Null
New-Item -ItemType Directory -Path "$testDir\documents" -Force | Out-Null

# Create text files
"Test text file content" | Out-File "$testDir\test.txt"
"Hello from document" | Out-File "$testDir\documents\doc1.txt"
"Another document" | Out-File "$testDir\documents\doc2.txt"

# Create a realistic JPEG image (small test image with JPEG header)
$jpegHeader = [byte[]](0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46)
$jpegData = [byte[]]::new(52000)  # ~50KB
(New-Object Random).NextBytes($jpegData)
$jpegFooter = [byte[]](0xFF, 0xD9)  # JPEG end marker
$jpegFull = $jpegHeader + $jpegData + $jpegFooter
[System.IO.File]::WriteAllBytes("$testDir\images\test_photo.jpg", $jpegFull)

# Create a PNG image (with PNG header)
$pngHeader = [byte[]](0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A)
$pngData = [byte[]]::new(35000)  # ~34KB
(New-Object Random).NextBytes($pngData)
$pngFull = $pngHeader + $pngData
[System.IO.File]::WriteAllBytes("$testDir\images\test_graphic.png", $pngFull)

# Create a PDF file (with PDF header)
$pdfContent = "%PDF-1.4`n1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj 2 0 obj<</Type/Pages/Count 1/Kids[3 0 R]>>endobj 3 0 obj<</Type/Page/MediaBox[0 0 612 792]/Parent 2 0 R/Resources<<>>>>endobj`nxref`n0 4`n0000000000 65535 f`n0000000009 00000 n`n0000000056 00000 n`n0000000115 00000 n`ntrailer<</Size 4/Root 1 0 R>>`nstartxref`n198`n%%EOF"
$pdfBytes = [System.Text.Encoding]::UTF8.GetBytes($pdfContent)
$pdfPadding = [byte[]]::new(100000)  # Pad to ~100KB
(New-Object Random).NextBytes($pdfPadding)
$pdfFull = $pdfBytes + $pdfPadding
[System.IO.File]::WriteAllBytes("$testDir\documents\test_document.pdf", $pdfFull)

# Create a large PDF
$largePdfData = [byte[]]::new(250000)  # ~244KB
(New-Object Random).NextBytes($largePdfData)
$largePdfFull = $pdfBytes + $largePdfData
[System.IO.File]::WriteAllBytes("$testDir\documents\large_report.pdf", $largePdfFull)

# Create an MP4 video file (with MP4 header)
$mp4Header = [byte[]](0x00, 0x00, 0x00, 0x20, 0x66, 0x74, 0x79, 0x70, 0x69, 0x73, 0x6F, 0x6D)
$mp4Data = [byte[]]::new(500000)  # ~488KB video
(New-Object Random).NextBytes($mp4Data)
$mp4Full = $mp4Header + $mp4Data
[System.IO.File]::WriteAllBytes("$testDir\videos\test_video.mp4", $mp4Full)

# Create a smaller video
$smallVideoData = [byte[]]::new(150000)  # ~146KB
(New-Object Random).NextBytes($smallVideoData)
$smallVideoFull = $mp4Header + $smallVideoData
[System.IO.File]::WriteAllBytes("$testDir\videos\short_clip.mp4", $smallVideoFull)

Write-Host "Test directory created with files:" -ForegroundColor Green
Get-ChildItem -Recurse $testDir | ForEach-Object { 
    if (-not $_.PSIsContainer) {
        Write-Host "  $($_.FullName) ($([math]::Round($_.Length/1KB, 2)) KB)" 
    }
}

# Build the project
Write-Host "`nBuilding wirego..." -ForegroundColor Yellow
go build -o wirego.exe .

if ($LASTEXITCODE -ne 0) {
    Write-Host "Build failed!" -ForegroundColor Red
    exit 1
}

Write-Host "Build successful!`n" -ForegroundColor Green

# Start sender in background (WebRTC mode - no -l flag)
Write-Host "Starting sender (WebRTC mode)..." -ForegroundColor Yellow
$sender = Start-Process -FilePath ".\wirego.exe" -ArgumentList "send", $testDir -PassThru -NoNewWindow -RedirectStandardOutput ".\sender_webrtc_output.txt"

# Wait for sender to connect to signaling server and get code
Start-Sleep -Seconds 3
$output = Get-Content ".\sender_webrtc_output.txt" -Raw

# Extract code (WebRTC codes are alphanumeric)
if ($output -match "Code:\s*(\w+)") {
    $code = $matches[1]
} else {
    Write-Host "Failed to extract code from sender output!" -ForegroundColor Red
    Write-Host "Output was:" -ForegroundColor Yellow
    Write-Host $output
    Stop-Process -Id $sender.Id -Force -ErrorAction SilentlyContinue
    exit 1
}

Write-Host "Sender started with WebRTC code: $code" -ForegroundColor Cyan
Write-Host ""

# Give sender time to connect to signaling server
Start-Sleep -Seconds 2

# Run receiver (WebRTC mode - no -l flag)
Write-Host "Starting receiver (WebRTC mode)..." -ForegroundColor Yellow
.\wirego.exe receive $code $receiveDir

# Check if sender is still running and stop it
if (-not $sender.HasExited) {
    Write-Host "`nStopping sender process..." -ForegroundColor Yellow
    Stop-Process -Id $sender.Id -Force -ErrorAction SilentlyContinue
}

# Verify received files
Write-Host "`n=== Verification ===" -ForegroundColor Cyan
if (Test-Path $receiveDir) {
    Write-Host "Received files:" -ForegroundColor Green
    Get-ChildItem -Recurse $receiveDir | ForEach-Object { 
        if (-not $_.PSIsContainer) {
            Write-Host "  $($_.FullName) ($([math]::Round($_.Length/1KB, 2)) KB)"
        }
    }
    
    # Compare file counts
    $sentFiles = Get-ChildItem -Recurse $testDir -File
    $receivedFiles = Get-ChildItem -Recurse $receiveDir -File
    $sentCount = $sentFiles.Count
    $receivedCount = $receivedFiles.Count
    
    Write-Host ""
    if ($sentCount -eq $receivedCount) {
        Write-Host "✓ TEST PASSED: All $sentCount files transferred successfully!" -ForegroundColor Green
        
        # Verify file sizes
        $allMatch = $true
        foreach ($sentFile in $sentFiles) {
            $relativePath = $sentFile.FullName.Replace($testDir, "").TrimStart("\")
            $receivedFile = Get-ChildItem -Recurse $receiveDir | Where-Object { $_.FullName -match [regex]::Escape($sentFile.Name) } | Select-Object -First 1
            
            if ($receivedFile -and $sentFile.Length -eq $receivedFile.Length) {
                Write-Host "  ✓ $($sentFile.Name): Size match ($($sentFile.Length) bytes)" -ForegroundColor Green
            } else {
                Write-Host "  ✗ $($sentFile.Name): Size mismatch!" -ForegroundColor Red
                $allMatch = $false
            }
        }
        
        if ($allMatch) {
            Write-Host "`n✓ All file sizes verified!" -ForegroundColor Green
        }
    } else {
        Write-Host "✗ TEST FAILED: Sent $sentCount files, received $receivedCount files" -ForegroundColor Red
    }
} else {
    Write-Host "✗ Receive directory not found!" -ForegroundColor Red
}

# Cleanup temp file
Remove-Item ".\sender_webrtc_output.txt" -Force -ErrorAction SilentlyContinue

Write-Host "`nTest complete!" -ForegroundColor Cyan
