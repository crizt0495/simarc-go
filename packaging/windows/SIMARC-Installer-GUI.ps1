param(
  [Parameter(Mandatory=$true)][string]$SetupScript,
  [string]$Mode = "",
  [string]$Hub = "",
  [switch]$NoDesktopShortcut
)
Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing
[System.Windows.Forms.Application]::EnableVisualStyles()

$form = New-Object Windows.Forms.Form
$form.Text = 'SIMARC - Arsip Record Center'
$form.StartPosition = 'CenterScreen'
$form.Size = New-Object Drawing.Size(560,380)
$form.MinimumSize = New-Object Drawing.Size(560,380)
$form.BackColor = [Drawing.Color]::White
$form.FormBorderStyle = 'FixedDialog'
$form.MaximizeBox = $false
$form.Icon = [Drawing.SystemIcons]::Application

$header = New-Object Windows.Forms.Panel
$header.Dock = 'Top'; $header.Height = 92
$header.BackColor = [Drawing.Color]::FromArgb(22,55,92)
$form.Controls.Add($header)
$title = New-Object Windows.Forms.Label
$title.Text = 'SIMARC'; $title.ForeColor = [Drawing.Color]::White
$title.Font = New-Object Drawing.Font('Segoe UI',24,[Drawing.FontStyle]::Bold)
$title.Location = New-Object Drawing.Point(28,14); $title.AutoSize = $true
$header.Controls.Add($title)
$sub = New-Object Windows.Forms.Label
$sub.Text = 'Arsip Record Center  •  Instalasi aman dan offline'
$sub.ForeColor = [Drawing.Color]::FromArgb(215,230,245)
$sub.Font = New-Object Drawing.Font('Segoe UI',10)
$sub.Location = New-Object Drawing.Point(31,56); $sub.AutoSize = $true
$header.Controls.Add($sub)

$intro = New-Object Windows.Forms.Label
$intro.Text = 'Pilih cara SIMARC digunakan pada komputer ini:'
$intro.Font = New-Object Drawing.Font('Segoe UI',11)
$intro.Location = New-Object Drawing.Point(30,115); $intro.AutoSize = $true
$form.Controls.Add($intro)
$offline = New-Object Windows.Forms.RadioButton
$offline.Text = 'Offline mandiri (direkomendasikan)'
$offline.Location = New-Object Drawing.Point(34,150); $offline.AutoSize = $true
$offline.Checked = $true; $form.Controls.Add($offline)
$client = New-Object Windows.Forms.RadioButton
$client.Text = 'Klien jaringan / server hub'
$client.Location = New-Object Drawing.Point(34,180); $client.AutoSize = $true
$form.Controls.Add($client)
$hubLabel = New-Object Windows.Forms.Label
$hubLabel.Text = 'Alamat hub (opsional):'; $hubLabel.Location = New-Object Drawing.Point(55,215); $hubLabel.AutoSize = $true
$form.Controls.Add($hubLabel)
$hubBox = New-Object Windows.Forms.TextBox
$hubBox.Location = New-Object Drawing.Point(190,211); $hubBox.Size = New-Object Drawing.Size(300,25)
$hubBox.Text = $Hub; $form.Controls.Add($hubBox)
$status = New-Object Windows.Forms.Label
$status.Text = 'Siap memasang SIMARC.'; $status.Location = New-Object Drawing.Point(34,260)
$status.AutoSize = $true; $form.Controls.Add($status)
$progress = New-Object Windows.Forms.ProgressBar
$progress.Location = New-Object Drawing.Point(34,285); $progress.Size = New-Object Drawing.Size(456,16)
$progress.Style = 'Marquee'; $progress.MarqueeAnimationSpeed = 25; $progress.Visible = $false
$form.Controls.Add($progress)
$install = New-Object Windows.Forms.Button
$install.Text = 'Pasang Sekarang'; $install.Location = New-Object Drawing.Point(350,318)
$install.Size = New-Object Drawing.Size(140,32)
$form.Controls.Add($install)
$cancel = New-Object Windows.Forms.Button
$cancel.Text = 'Batal'; $cancel.Location = New-Object Drawing.Point(260,318)
$cancel.Size = New-Object Drawing.Size(80,32); $cancel.DialogResult = 'Cancel'
$form.Controls.Add($cancel)
$form.AcceptButton = $install; $form.CancelButton = $cancel

$install.Add_Click({
  $install.Enabled = $false; $cancel.Enabled = $false
  $offline.Enabled = $false; $client.Enabled = $false; $hubBox.Enabled = $false
  $progress.Visible = $true; $status.Text = 'Menyalin berkas dan menyiapkan database...'
  $chosen = if ($offline.Checked) { 'offline' } else { 'client' }
  $args = @('-NoProfile','-ExecutionPolicy','Bypass','-File',$SetupScript,'-Mode',$chosen,'-Quiet')
  if ($hubBox.Text.Trim()) { $args += @('-Hub',$hubBox.Text.Trim()) }
  if ($NoDesktopShortcut) { $args += '-NoDesktopShortcut' }
  try {
    $psi = New-Object System.Diagnostics.ProcessStartInfo
    $psi.FileName = 'powershell.exe'; $psi.Arguments = ($args | ForEach-Object { '"' + $_ + '"' }) -join ' '
    $psi.UseShellExecute = $false; $psi.CreateNoWindow = $true
    $p = [System.Diagnostics.Process]::Start($psi)
    $p.WaitForExit()
    if ($p.ExitCode -ne 0) { throw "Installer mengembalikan kode $($p.ExitCode)" }
    $progress.Visible = $false; $status.Text = 'Instalasi selesai. SIMARC siap digunakan.'
    $install.Text = 'Buka SIMARC'; $install.Enabled = $true
    $install.Add_Click({ Start-Process (Join-Path $env:LOCALAPPDATA 'SIMARC\run.bat'); $form.Close() })
  } catch {
    $progress.Visible = $false; $status.Text = 'Instalasi gagal.'
    [Windows.Forms.MessageBox]::Show($form, $_.Exception.Message, 'SIMARC', 'OK', 'Error')
    $cancel.Enabled = $true; $install.Enabled = $true
  }
})
[void]$form.ShowDialog()
