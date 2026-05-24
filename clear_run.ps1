Get-ChildItem -Path "run" -Recurse -Filter "*.json" | Remove-Item -Force
Write-Host "Deleted all JSON files in run/"
