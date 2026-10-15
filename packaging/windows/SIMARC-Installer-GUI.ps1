param(
  [Parameter(Mandatory=$true)][string]$SetupScript,
  [string]$Mode = "",
  [string]$Hub = "",
  [string]$AppName = "SIMARC",
  [switch]$NoDesktopShortcut
)
Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing
[System.Windows.Forms.Application]::EnableVisualStyles()

# ── Palet warna ─────────────────────────────────────────────────────────────
$cNavy   = [Drawing.Color]::FromArgb(15, 42, 74)      # header gelap
$cNavy2  = [Drawing.Color]::FromArgb(24, 62, 108)     # gradasi
$cAccent = [Drawing.Color]::FromArgb(0, 158, 224)     # biru terang
$cGreen  = [Drawing.Color]::FromArgb(46, 160, 90)     # sukses
$cText   = [Drawing.Color]::FromArgb(38, 45, 56)
$cMuted  = [Drawing.Color]::FromArgb(120, 132, 146)
$cCard   = [Drawing.Color]::White
$cBg     = [Drawing.Color]::FromArgb(244, 247, 251)

$fTitle = New-Object Drawing.Font('Segoe UI', 26, [Drawing.FontStyle]::Bold)
$fSub   = New-Object Drawing.Font('Segoe UI', 10)
$fHead  = New-Object Drawing.Font('Segoe UI', 13, [Drawing.FontStyle]::Bold)
$fBody  = New-Object Drawing.Font('Segoe UI', 10.5)
$fSmall = New-Object Drawing.Font('Segoe UI', 9)
$fBtn   = New-Object Drawing.Font('Segoe UI', 10.5, [Drawing.FontStyle]::Bold)

# ── Kontroler kartu mode (radio ala kartu) ──────────────────────────────────
function New-ModeCard {
  param([string]$Title, [string]$Desc, [int]$Y, [bool]$Selected)
  $panel = New-Object Windows.Forms.Panel
  $panel.Location = New-Object Drawing.Point(34, $Y)
  $panel.Size = New-Object Drawing.Size(456, 62)
  $panel.BackColor = if ($Selected) { [Drawing.Color]::FromArgb(233, 245, 253) } else { $cCard }
  $panel.BorderStyle = if ($Selected) { 'FixedSingle' } else { 'FixedSingle' }

  $lblT = New-Object Windows.Forms.Label
  $lblT.Text = $Title; $lblT.Font = $fHead; $lblT.ForeColor = $cText
  $lblT.Location = New-Object Drawing.Point(16, 10); $lblT.AutoSize = $true
  $panel.Controls.Add($lblT)

  $lblD = New-Object Windows.Forms.Label
  $lblD.Text = $Desc; $lblD.Font = $fSmall; $lblD.ForeColor = $cMuted
  $lblD.Location = New-Object Drawing.Point(17, 36); $lblD.AutoSize = $true
  $panel.Controls.Add($lblD)

  return $panel
}

# ── Formulir ────────────────────────────────────────────────────────────────
$form = New-Object Windows.Forms.Form
$form.Text = "$AppName - Instalasi"
$form.StartPosition = 'CenterScreen'
$form.ClientSize = New-Object Drawing.Size(540, 420)
$form.FormBorderStyle = 'FixedDialog'
$form.MaximizeBox = $false; $form.MinimizeBox = $false
$form.BackColor = $cBg
$form.Font = $fBody
try { $form.Icon = [Drawing.SystemIcons]::Shield } catch { }

# ── Header bergradasi ───────────────────────────────────────────────────────
$header = New-Object Windows.Forms.Panel
$header.Dock = 'Top'; $header.Height = 104; $header.BackColor = $cNavy
$form.Controls.Add($header)
$header.Add_Paint({
  param($s, $e)
  $rect = $s.ClientRectangle
  if ($rect.Width -le 0 -or $rect.Height -le 0) { return }
  $brush = New-Object Drawing.Drawing2D.LinearGradientBrush(
    $rect, $cNavy, $cNavy2, [Drawing.Drawing2D.LinearGradientMode]::Horizontal)
  $e.Graphics.FillRectangle($brush, $rect)
  $e.Graphics.FillEllipse((New-Object Drawing.SolidBrush ([Drawing.Color]::FromArgb(30, 255, 255, 255))), 430, -40, 180, 180)
  $brush.Dispose()
})

$badge = New-Object Windows.Forms.Label
$badge.Text = "ARSIP RECORD CENTER"
$badge.ForeColor = $cAccent; $badge.Font = New-Object Drawing.Font('Segoe UI', 8.5, [Drawing.FontStyle]::Bold)
$badge.Location = New-Object Drawing.Point(34, 16); $badge.AutoSize = $true
$header.Controls.Add($badge)

$title = New-Object Windows.Forms.Label
$title.Text = $AppName; $title.ForeColor = [Drawing.Color]::White; $title.Font = $fTitle
$title.Location = New-Object Drawing.Point(32, 32); $title.AutoSize = $true
$header.Controls.Add($title)

$sub = New-Object Windows.Forms.Label
$sub.Text = 'Instalasi aman, offline, dan siap sinkron antar-komputer'
$sub.ForeColor = [Drawing.Color]::FromArgb(200, 220, 240); $sub.Font = $fSub
$sub.Location = New-Object Drawing.Point(36, 74); $sub.AutoSize = $true
$header.Controls.Add($sub)

# ── Konten (panel yang bisa berganti halaman) ───────────────────────────────
$body = New-Object Windows.Forms.Panel
$body.Location = New-Object Drawing.Point(0, 104)
$body.Size = New-Object Drawing.Size(540, 316)
$body.BackColor = $cBg
$form.Controls.Add($body)

# ---------- HALAMAN 1: pilih mode ----------
$page1 = New-Object Windows.Forms.Panel
$page1.Dock = 'Fill'; $page1.BackColor = $cBg
$body.Controls.Add($page1)

$intro = New-Object Windows.Forms.Label
$intro.Text = 'Bagaimana komputer ini akan menggunakan SIMARC?'
$intro.Font = $fHead; $intro.ForeColor = $cText
$intro.Location = New-Object Drawing.Point(34, 18); $intro.AutoSize = $true
$page1.Controls.Add($intro)

$cardOffline = New-ModeCard -Title 'Offline Mandiri  (Disarankan)' `
  -Desc 'Database otomatis berjalan di latar belakang. Langsung pakai tanpa server.' -Y 56 -Selected $true
$page1.Controls.Add($cardOffline)
$cardClient = New-ModeCard -Title 'Klien Jaringan' `
  -Desc 'Terhubung ke server pusat (hub) di jaringan kantor.' -Y 126 -Selected $false
$page1.Controls.Add($cardClient)

$rbOffline = New-Object Windows.Forms.RadioButton
$rbOffline.Text = ''; $rbOffline.Checked = $true
$rbOffline.Location = New-Object Drawing.Point(6, 22); $rbOffline.Size = New-Object Drawing.Size(16, 16)
$cardOffline.Controls.Add($rbOffline)
$rbClient = New-Object Windows.Forms.RadioButton
$rbClient.Text = ''
$rbClient.Location = New-Object Drawing.Point(6, 22); $rbClient.Size = New-Object Drawing.Size(16, 16)
$cardClient.Controls.Add($rbClient)

$lblHub = New-Object Windows.Forms.Label
$lblHub.Text = 'Alamat server hub:'; $lblHub.Font = $fBody; $lblHub.ForeColor = $cMuted
$lblHub.Location = New-Object Drawing.Point(36, 205); $lblHub.AutoSize = $true
$page1.Controls.Add($lblHub)
$hubBox = New-Object Windows.Forms.TextBox
$hubBox.Location = New-Object Drawing.Point(36, 228); $hubBox.Size = New-Object Drawing.Size(454, 26)
$hubBox.Font = $fBody; $hubBox.Text = $Hub
$hubBox.Enabled = $false; $page1.Controls.Add($hubBox)

function Select-Card([bool]$offlineOn) {
  $rbOffline.Checked = $offlineOn; $rbClient.Checked = -not $offlineOn
  $cardOffline.BackColor = if ($offlineOn) { [Drawing.Color]::FromArgb(233, 245, 253) } else { $cCard }
  $cardClient.BackColor  = if ($offlineOn) { $cCard } else { [Drawing.Color]::FromArgb(233, 245, 253) }
  $hubBox.Enabled = -not $offlineOn
}
$cardOffline.Add_Click({ Select-Card $true })
$cardClient.Add_Click({ Select-Card $false })
$rbOffline.Add_CheckedChanged({ if ($rbOffline.Checked) { Select-Card $true } })
$rbClient.Add_CheckedChanged({ if ($rbClient.Checked) { Select-Card $false } })

$btnInstall = New-Object Windows.Forms.Button
$btnInstall.Text = 'Pasang Sekarang'
$btnInstall.Font = $fBtn; $btnInstall.Size = New-Object Drawing.Size(160, 40)
$btnInstall.Location = New-Object Drawing.Point(330, 264)
$btnInstall.FlatStyle = 'Flat'; $btnInstall.FlatAppearance.BorderSize = 0
$btnInstall.BackColor = $cAccent; $btnInstall.ForeColor = [Drawing.Color]::White
$btnInstall.Cursor = 'Hand'; $page1.Controls.Add($btnInstall)

$btnCancel = New-Object Windows.Forms.Button
$btnCancel.Text = 'Batal'; $btnCancel.Font = $fBody
$btnCancel.Size = New-Object Drawing.Size(90, 40)
$btnCancel.Location = New-Object Drawing.Point(232, 264)
$btnCancel.FlatStyle = 'Flat'; $btnCancel.FlatAppearance.BorderColor = [Drawing.Color]::FromArgb(205, 212, 220)
$btnCancel.BackColor = $cCard; $btnCancel.ForeColor = $cText
$btnCancel.Cursor = 'Hand'; $btnCancel.DialogResult = 'Cancel'
$page1.Controls.Add($btnCancel)
$form.AcceptButton = $btnInstall; $form.CancelButton = $btnCancel

# ---------- HALAMAN 2: progres ----------
$page2 = New-Object Windows.Forms.Panel
$page2.Dock = 'Fill'; $page2.BackColor = $cBg; $page2.Visible = $false
$body.Controls.Add($page2)

$progTitle = New-Object Windows.Forms.Label
$progTitle.Text = 'Memasang SIMARC...'; $progTitle.Font = $fHead; $progTitle.ForeColor = $cText
$progTitle.Location = New-Object Drawing.Point(34, 60); $progTitle.AutoSize = $true
$page2.Controls.Add($progTitle)

$progStep = New-Object Windows.Forms.Label
$progStep.Text = 'Menyiapkan berkas instalasi...'; $progStep.Font = $fBody; $progStep.ForeColor = $cMuted
$progStep.Location = New-Object Drawing.Point(34, 92); $progStep.AutoSize = $true
$page2.Controls.Add($progStep)

$progBar = New-Object Windows.Forms.ProgressBar
$progBar.Location = New-Object Drawing.Point(34, 130); $progBar.Size = New-Object Drawing.Size(472, 18)
$progBar.Style = 'Marquee'; $progBar.MarqueeAnimationSpeed = 30
$page2.Controls.Add($progBar)

$steps = @(
  'Menyiapkan berkas instalasi',
  'Menyalin komponen aplikasi',
  'Memasang database portabel',
  'Mengonfigurasi aplikasi',
  'Membuat pintasan & registrasi'
)
$stepList = New-Object Windows.Forms.ListBox
$stepList.Location = New-Object Drawing.Point(34, 165); $stepList.Size = New-Object Drawing.Size(472, 90)
$stepList.BorderStyle = 'None'; $stepList.BackColor = $cBg; $stepList.Font = $fBody
$stepList.ForeColor = $cMuted; $stepList.SelectionMode = 'None'
foreach ($s in $steps) { [void]$stepList.Items.Add("   $s") }
$page2.Controls.Add($stepList)

# ---------- HALAMAN 3: selesai ----------
$page3 = New-Object Windows.Forms.Panel
$page3.Dock = 'Fill'; $page3.BackColor = $cBg; $page3.Visible = $false
$body.Controls.Add($page3)

$doneIcon = New-Object Windows.Forms.Label
$doneIcon.Text = '✓'; $doneIcon.Font = New-Object Drawing.Font('Segoe UI', 44, [Drawing.FontStyle]::Bold)
$doneIcon.ForeColor = $cGreen; $doneIcon.Location = New-Object Drawing.Point(34, 34); $doneIcon.AutoSize = $true
$page3.Controls.Add($doneIcon)

$doneTitle = New-Object Windows.Forms.Label
$doneTitle.Text = 'Instalasi Berhasil!'; $doneTitle.Font = New-Object Drawing.Font('Segoe UI',16,[Drawing.FontStyle]::Bold)
$doneTitle.ForeColor = $cText; $doneTitle.Location = New-Object Drawing.Point(96, 44); $doneTitle.AutoSize = $true
$page3.Controls.Add($doneTitle)

$doneDesc = New-Object Windows.Forms.Label
$doneDesc.Text = 'SIMARC siap digunakan. Anda dapat membukanya sekarang atau lewat pintasan Desktop.'
$doneDesc.Font = $fBody; $doneDesc.ForeColor = $cMuted
$doneDesc.Location = New-Object Drawing.Point(98, 80); $doneDesc.MaximumSize = New-Object Drawing.Size(400,0)
$page3.Controls.Add($doneDesc)

$btnOpen = New-Object Windows.Forms.Button
$btnOpen.Text = 'Buka SIMARC'; $btnOpen.Font = $fBtn
$btnOpen.Size = New-Object Drawing.Size(160, 42)
$btnOpen.Location = New-Object Drawing.Point(330, 200)
$btnOpen.FlatStyle = 'Flat'; $btnOpen.FlatAppearance.BorderSize = 0
$btnOpen.BackColor = $cGreen; $btnOpen.ForeColor = [Drawing.Color]::White
$btnOpen.Cursor = 'Hand'; $page3.Controls.Add($btnOpen)

$btnClose = New-Object Windows.Forms.Button
$btnClose.Text = 'Tutup'; $btnClose.Font = $fBody
$btnClose.Size = New-Object Drawing.Size(90, 42)
$btnClose.Location = New-Object Drawing.Point(232, 200)
$btnClose.FlatStyle = 'Flat'; $btnClose.FlatAppearance.BorderColor = [Drawing.Color]::FromArgb(205, 212, 220)
$btnClose.BackColor = $cCard; $btnClose.ForeColor = $cText
$btnClose.Cursor = 'Hand'; $btnClose.DialogResult = 'Cancel'
$page3.Controls.Add($btnClose)

# ── Logika tombol Pasang ─────────────────────────────────────────────────────
$btnInstall.Add_Click({
  $page1.Visible = $false; $page2.Visible = $true

  $chosen = if ($rbOffline.Checked) { 'offline' } else { 'client' }
  $psArgs = @('-NoProfile','-ExecutionPolicy','Bypass','-File',$SetupScript,'-Mode',$chosen,'-Quiet')
  if (-not $rbOffline.Checked -and $hubBox.Text.Trim()) { $psArgs += @('-Hub', $hubBox.Text.Trim()) }
  if ($NoDesktopShortcut) { $psArgs += '-NoDesktopShortcut' }

  # Animasi langkah: tandai berjalan -> selesai secara bertahap.
  $timer = New-Object Windows.Forms.Timer
  $timer.Interval = 550
  $idx = 0
  $timer.Add_Tick({
    if ($idx -lt $script:stepList.Items.Count) {
      $cur = $script:stepList.Items[$idx].ToString().Trim()
      $script:stepList.Items[$idx] = "   $([char]0x2713)  $cur"
      $script:progStep.Text = "$cur..."
      $idx++
    }
  })
  $timer.Start()

  try {
    $psi = New-Object System.Diagnostics.ProcessStartInfo
    $psi.FileName = 'powershell.exe'
    $psi.Arguments = ($psArgs | ForEach-Object { '"' + $_ + '"' }) -join ' '
    $psi.UseShellExecute = $false; $psi.CreateNoWindow = $true
    $psi.RedirectStandardError = $true; $psi.RedirectStandardOutput = $true
    $proc = [System.Diagnostics.Process]::Start($psi)
    $errOut = $proc.StandardError.ReadToEnd()
    $stdOut = $proc.StandardOutput.ReadToEnd()
    $proc.WaitForExit()

    $timer.Stop()
    for ($k = 0; $k -lt $stepList.Items.Count; $k++) {
      $cur = $stepList.Items[$k].ToString().TrimStart([char]0x2713, ' ')
      $stepList.Items[$k] = "   $([char]0x2713)  $cur"
    }

    if ($proc.ExitCode -ne 0) {
      # Skrip mengembalikan non-zero; tetap tampilkan jika folder terpasang.
      $installed = Test-Path (Join-Path $env:LOCALAPPDATA 'SIMARC\run.bat')
      if (-not $installed) { throw "Installer mengembalikan kode $($proc.ExitCode). $errOut" }
    }

    $page2.Visible = $false; $page3.Visible = $true
  } catch {
    $timer.Stop()
    $page2.Visible = $false
    [Windows.Forms.MessageBox]::Show($form, $_.Exception.Message, "$AppName - Kesalahan", 'OK', 'Error') | Out-Null
    $page1.Visible = $true
  }
})

$btnOpen.Add_Click({
  $bat = Join-Path $env:LOCALAPPDATA 'SIMARC\run.bat'
  if (Test-Path $bat) { Start-Process $bat }
  $form.Close()
})
$btnClose.Add_Click({ $form.Close() })

[void]$form.ShowDialog()
