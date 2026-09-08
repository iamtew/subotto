# Package Subotto for Linux VPS deploy (run from repo root via `just package-linux`).
$ErrorActionPreference = 'Stop'

if (-not (Test-Path 'bin/subotto-linux')) {
    throw 'bin/subotto-linux missing — run build-linux first'
}

if (Test-Path 'dist') {
    Remove-Item -Recurse -Force 'dist'
}

$stage = 'dist/stage'
New-Item -ItemType Directory -Force -Path $stage, "$stage/webroot", "$stage/deploy", "$stage/docs" | Out-Null

Copy-Item 'bin/subotto-linux' $stage/
Copy-Item -Recurse 'webroot/*' "$stage/webroot/"
Copy-Item '.env.example' $stage/
Copy-Item 'deploy/subotto.service' "$stage/deploy/"
Copy-Item 'deploy/README-DEPLOY.txt' "$stage/README-DEPLOY.txt"
Copy-Item 'docs/DEPLOY.md' "$stage/docs/"

Compress-Archive -Path "$stage/*" -DestinationPath 'dist/subotto-linux.zip' -Force
Remove-Item -Recurse -Force $stage

Write-Host 'Wrote dist/subotto-linux.zip'
