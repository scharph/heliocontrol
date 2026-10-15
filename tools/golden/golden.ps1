# Regenerates internal/frame/testdata/golden.txt from the vendor's original
# FlipTools DataFrame.dll (shipped with homeControl). Windows PowerShell 5.1 only.
#
#   powershell -File tools/golden/golden.ps1 -Dll "C:\path\to\homeControl APP\DataFrame.dll"
param(
  [Parameter(Mandatory = $true)][string]$Dll,
  [string]$Out
)
if (-not $Out) { $Out = Join-Path $PSScriptRoot '..\..\internal\frame\testdata\golden.txt' }
# Load from bytes: the shipped DLL usually carries a Mark-of-the-Web that blocks LoadFrom.
$asm = [Reflection.Assembly]::Load([IO.File]::ReadAllBytes((Resolve-Path $Dll)))
$df = $asm.CreateInstance('FlipTools.DataFrame')
function Hex([byte[]]$b) { if ($null -eq $b) { return '-' }; -join ($b | ForEach-Object { '{0:x2}' -f $_ }) }
$ascii = [Text.Encoding]::ASCII
$payloads = New-Object System.Collections.Generic.List[byte[]]
foreach ($s in '~LIN;', '~LOUT;', '~RM;', '~RID;', '~SP,NR=13,VAL=21.5;', '~SP,NR=13,VAL=1.5', '~CLK,DA=15.10.26,TI=12:30:00,WD=4;') {
  $payloads.Add($ascii.GetBytes($s))
}
for ($i = 0; $i -lt 400; $i++) { $payloads.Add($ascii.GetBytes("~MP,NR=$i;")) }
$payloads.Add([byte[]](0x7e, 0x02, 0x2b, 0x11, 0x13, 0xfe, 0x00, 0xc5, 0x3b))
$long = New-Object byte[] 300
for ($i = 0; $i -lt 300; $i++) { $long[$i] = [byte](0x41 + ($i % 26)) }
$payloads.Add($long)
$lines = foreach ($p in $payloads) {
  $frame = $df.Create($p, 0)
  $buf = [byte[]]$frame.Clone()
  $dec = $df.ProcessSingle([ref]$buf)
  '{0} {1} {2}' -f (Hex $p), (Hex $frame), (Hex $dec)
}
New-Item -ItemType Directory -Force (Split-Path $Out) | Out-Null
[IO.File]::WriteAllLines($Out, [string[]]$lines)
"wrote $($lines.Count) vectors to $Out"
