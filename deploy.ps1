$Server = "root@47.242.41.252"
$Key    = "C:\Users\Administrator\.ssh\abc.pem"
$Ts     = Get-Date -Format yyyyMMddHHmmss

Write-Host "Building frontend..."
cd frontend
npm run build
cd ..

Write-Host "Building Linux backend..."
$env:GOOS='linux'
$env:GOARCH='amd64'
go build -o timeapp_linux .
Remove-Item Env:GOOS
Remove-Item Env:GOARCH

Write-Host "Packaging frontend..."
tar -czf dist_upload.tgz -C frontend dist

Write-Host "Creating release directory on server..."
ssh -o StrictHostKeyChecking=no -i $Key $Server "mkdir -p /opt/time/releases/$Ts/frontend"

Write-Host "Uploading files..."
scp -o StrictHostKeyChecking=no -i $Key timeapp_linux "$($Server):/opt/time/releases/$Ts/timeapp"
scp -o StrictHostKeyChecking=no -i $Key dist_upload.tgz "$($Server):/opt/time/releases/$Ts/dist_upload.tgz"

Write-Host "Extracting frontend on server..."
ssh -o StrictHostKeyChecking=no -i $Key $Server "cd /opt/time/releases/$Ts && tar -xzf dist_upload.tgz -C frontend && rm -f dist_upload.tgz"

Write-Host "Switching version and restarting service..."
ssh -o StrictHostKeyChecking=no -i $Key $Server "set -e; install -m 755 /opt/time/releases/$Ts/timeapp /opt/time/timeapp; rm -rf /opt/time/frontend/dist; cp -a /opt/time/releases/$Ts/frontend/dist /opt/time/frontend/dist; chown -R timeapp:timeapp /opt/time/timeapp /opt/time/frontend/dist; systemctl restart timeapp; systemctl is-active timeapp"

Write-Host "Deployment completed successfully!"
