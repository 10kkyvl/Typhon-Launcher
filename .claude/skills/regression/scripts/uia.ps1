# Minimal UI Automation helpers for native Win32 dialogs of the launcher
# (file and folder pickers, message boxes) that CDP cannot see.
#
#   powershell -NoProfile -File uia.ps1 windows [-Process typhon]
#   powershell -NoProfile -File uia.ps1 controls [-Window <title part>]
#   powershell -NoProfile -File uia.ps1 set-filename <path> [-Window <title part>]
#   powershell -NoProfile -File uia.ps1 invoke <automationId> [-Window <title part>]
#
# In a common file dialog AutomationId 1148 is the file name box (a ComboBox
# or Edit), in a folder picker the "Folder:" Edit is 1152; set-filename tries
# both. The Button with AutomationId 1 is Open/Save/Select Folder and the
# Cancel Button is 2. invoke only matches Buttons: list items carry small
# numeric ids too.
# Without -Window the target is the first #32770 dialog of the process.
# Exit codes: 0 ok, 1 nothing found or the action failed, 2 usage.
#
# Only top-level windows and their direct window children are listed: walking
# into the main window would make WebView2 build its whole accessibility tree.

param(
    [Parameter(Position = 0)][string]$Command,
    [Parameter(Position = 1)][string]$Arg,
    [string]$Window,
    [string]$Process = 'typhon'
)

$ErrorActionPreference = 'Stop'

try {
    [Console]::OutputEncoding = [Text.UTF8Encoding]::new($false)
} catch {
    # No console handle when stdout is redirected; the output is bytes then anyway.
}

Add-Type -AssemblyName UIAutomationClient, UIAutomationTypes

function Fail([int]$Code, [string]$Message) {
    [Console]::Error.WriteLine("error: $Message")
    exit $Code
}

function Get-ProcessWindows {
    $ids = @(Get-Process -Name $Process -ErrorAction SilentlyContinue | ForEach-Object { $_.Id })
    if ($ids.Count -eq 0) { Fail 1 "no running process named '$Process'" }
    $isWindow = New-Object Windows.Automation.PropertyCondition ([Windows.Automation.AutomationElement]::ControlTypeProperty), ([Windows.Automation.ControlType]::Window)
    $found = @()
    foreach ($top in [Windows.Automation.AutomationElement]::RootElement.FindAll('Children', $isWindow)) {
        if ($ids -notcontains $top.Current.ProcessId) { continue }
        $found += $top
        foreach ($child in $top.FindAll('Children', $isWindow)) { $found += $child }
    }
    return $found
}

function Get-Dialog {
    $all = @(Get-ProcessWindows)
    if ($Window) {
        $hit = @($all | Where-Object { $_.Current.Name.IndexOf($Window, [StringComparison]::OrdinalIgnoreCase) -ge 0 })
        if ($hit.Count -eq 0) { Fail 1 "no window of '$Process' with a title containing '$Window'" }
        return $hit[0]
    }
    $hit = @($all | Where-Object { $_.Current.ClassName -eq '#32770' })
    if ($hit.Count -eq 0) { Fail 1 "no #32770 dialog among the windows of '$Process'; run 'windows' or pass -Window" }
    return $hit[0]
}

function Find-ById($Root, [string]$Id, $Type) {
    $byId = New-Object Windows.Automation.PropertyCondition ([Windows.Automation.AutomationElement]::AutomationIdProperty), $Id
    $byType = New-Object Windows.Automation.PropertyCondition ([Windows.Automation.AutomationElement]::ControlTypeProperty), $Type
    $el = $Root.FindFirst('Descendants', (New-Object Windows.Automation.AndCondition $byId, $byType))
    if ($null -eq $el) { Fail 1 "no $($Type.ProgrammaticName) with AutomationId '$Id' in '$($Root.Current.Name)'; list them with 'controls'" }
    return $el
}

switch ($Command) {
    'windows' {
        $all = @(Get-ProcessWindows)
        if ($all.Count -eq 0) { Fail 1 "'$Process' has no UI Automation windows" }
        foreach ($w in $all) {
            $c = $w.Current
            $r = $c.BoundingRectangle
            Write-Output ("pid={0} hwnd=0x{1:X} class={2} name=""{3}"" rect={4},{5} {6}x{7}" -f $c.ProcessId, $c.NativeWindowHandle, $c.ClassName, $c.Name, [int]$r.X, [int]$r.Y, [int]$r.Width, [int]$r.Height)
        }
    }
    'controls' {
        $dlg = Get-Dialog
        foreach ($el in $dlg.FindAll('Descendants', [Windows.Automation.Condition]::TrueCondition)) {
            $c = $el.Current
            Write-Output ("id={0} type={1} name=""{2}""" -f $c.AutomationId, $c.ControlType.ProgrammaticName.Replace('ControlType.', ''), $c.Name)
        }
    }
    'set-filename' {
        if (-not $Arg) { Fail 2 'usage: set-filename <path> [-Window <title part>]' }
        $dlg = Get-Dialog
        $box = $null
        foreach ($id in @('1148', '1152')) {
            foreach ($type in @([Windows.Automation.ControlType]::ComboBox, [Windows.Automation.ControlType]::Edit)) {
                $byId = New-Object Windows.Automation.PropertyCondition ([Windows.Automation.AutomationElement]::AutomationIdProperty), $id
                $byType = New-Object Windows.Automation.PropertyCondition ([Windows.Automation.AutomationElement]::ControlTypeProperty), $type
                $box = $dlg.FindFirst('Descendants', (New-Object Windows.Automation.AndCondition $byId, $byType))
                if ($null -ne $box) { break }
            }
            if ($null -ne $box) { break }
        }
        if ($null -eq $box) { Fail 1 "no ComboBox or Edit with AutomationId 1148 or 1152 in '$($dlg.Current.Name)'; list the controls with 'controls'" }
        $pattern = $null
        if (-not $box.TryGetCurrentPattern([Windows.Automation.ValuePattern]::Pattern, [ref]$pattern)) {
            $edit = $box.FindFirst('Descendants', (New-Object Windows.Automation.PropertyCondition ([Windows.Automation.AutomationElement]::ControlTypeProperty), ([Windows.Automation.ControlType]::Edit)))
            if ($null -eq $edit -or -not $edit.TryGetCurrentPattern([Windows.Automation.ValuePattern]::Pattern, [ref]$pattern)) {
                Fail 1 'the file name box (AutomationId 1148) exposes no ValuePattern'
            }
        }
        $pattern.SetValue($Arg)
        Write-Output ("set {0} in ""{1}"" to ""{2}""" -f $box.Current.AutomationId, $dlg.Current.Name, $pattern.Current.Value)
    }
    'invoke' {
        if (-not $Arg) { Fail 2 'usage: invoke <automationId> [-Window <title part>]' }
        $dlg = Get-Dialog
        $btn = Find-ById $dlg $Arg ([Windows.Automation.ControlType]::Button)
        $pattern = $null
        if (-not $btn.TryGetCurrentPattern([Windows.Automation.InvokePattern]::Pattern, [ref]$pattern)) {
            Fail 1 "control '$Arg' ($($btn.Current.Name)) exposes no InvokePattern"
        }
        $line = "invoked {0} ""{1}"" in ""{2}""" -f $Arg, $btn.Current.Name, $dlg.Current.Name
        $pattern.Invoke()
        Write-Output $line
    }
    default {
        Fail 2 'usage: uia.ps1 <windows|controls|set-filename|invoke> [arg] [-Window <title part>] [-Process <name>]'
    }
}
