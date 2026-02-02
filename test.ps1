    # Create test directory structure
    $testDir = ".\test"
    $receiveDir = ".\test_received"

    # Clean up previous test
    if (Test-Path $testDir) { Remove-Item -Recurse -Force $testDir }
    if (Test-Path $receiveDir) { Remove-Item -Recurse -Force $receiveDir }

    # Create test structure
    New-Item -ItemType Directory -Path "$testDir\subdir1" -Force | Out-Null
    New-Item -ItemType Directory -Path "$testDir\subdir2" -Force | Out-Null

    # Create test files with some content
    "Hello from file1" | Out-File "$testDir\file1.txt"
    "Hello from file2" | Out-File "$testDir\file2.txt"
    "Hello from file3" | Out-File "$testDir\file3.txt"
    "Nested file in subdir1" | Out-File "$testDir\subdir1\nested1.txt"
    "Nested file in subdir2" | Out-File "$testDir\subdir2\nested2.txt"

    Write-Host "Test directory created:" -ForegroundColor Green
    Get-ChildItem -Recurse $testDir | ForEach-Object { Write-Host "  $($_.FullName)" }

    # Build the project
    Write-Host "`ni am building wirego..." -ForegroundColor Yellow
    go build -o wirego.exe .

    if ($LASTEXITCODE -ne 0) {
        Write-Host "Build failed!" -ForegroundColor Red
        exit 1
    }

    Write-Host "build successful!`n" -ForegroundColor Green
    `
    # Start sender in background (WebRTC mode - no -l flag)
    Write-Host "starting sender..." -ForegroundColor Yellow
    $sender = Start-Process -FilePath ".\wirego.exe" -ArgumentList "send", $testDir -PassThru -NoNewWindow -RedirectStandardOutput ".\sender_output.txt"

    # Wait for server to start and extract code from output (WebRTC codes are alphanumeric)
    Start-Sleep -Seconds 3
    $output = Get-Content ".\sender_output.txt" -Raw
    
    # Extract alphanumeric code for WebRTC
    if ($output -match "Code:\s*(\w+)") {
        $code = $matches[1]
    } else {
        Write-Host "Failed to get code from sender!" -ForegroundColor Red
        Stop-Process -Id $sender.Id -Force
        exit 1
    }

    Write-Host "sender started with code: $code" -ForegroundColor Cyan

    # Give sender time to connect to signaling server
    Start-Sleep -Seconds 2

    # Run receiver (WebRTC mode - no -l flag)
    Write-Host "`nstarting receiver..." -ForegroundColor Yellow
    .\wirego.exe receive $code $receiveDir 
        
    # Verify received files
    Write-Host "`nverifying received files:" -ForegroundColor Yellow
    if (Test-Path $receiveDir) {
        Get-ChildItem -Recurse $receiveDir | ForEach-Object { Write-Host "  $($_.FullName)" -ForegroundColor Green }
        
        # Compare file counts
        $sentCount = (Get-ChildItem -Recurse $testDir -File).Count
        $receivedCount = (Get-ChildItem -Recurse $receiveDir -File).Count
        
        if ($sentCount -ne $receivedCount) {
            Write-Host "`nTEST FAILED: Sent $sentCount files, received $receivedCount" -ForegroundColor Red
        } else {
            Write-Host "`nTEST PASSED: All $sentCount files transferred!" -ForegroundColor Green
        }
    } else {
        Write-Host "Receive directory not found!" -ForegroundColor Red
    }

    # Cleanup temp file
    Remove-Item ".\sender_output.txt" -Force -ErrorAction SilentlyContinue  