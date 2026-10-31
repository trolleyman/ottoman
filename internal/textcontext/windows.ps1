$ErrorActionPreference = 'Stop'
[Console]::InputEncoding = New-Object System.Text.UTF8Encoding($false)
[Console]::OutputEncoding = New-Object System.Text.UTF8Encoding($false)
Add-Type -AssemblyName UIAutomationClient,UIAutomationTypes
$references = @([System.Windows.Automation.AutomationElement].Assembly.Location, [System.Windows.Automation.Text.TextPatternRange].Assembly.Location)
Add-Type -ReferencedAssemblies $references -TypeDefinition @'
using System;
using System.Threading;
using System.Runtime.InteropServices;
using System.Windows.Automation;
using System.Windows.Automation.Text;
public class OttomanText {
 public class Context {
  public string Identity="", Value="";
  public int Start, End, WindowStart;
  public bool Editable, Multiline, Available, Protected, Password;
 }
 static long epoch;
 static AutomationFocusChangedEventHandler focus = delegate(object s, AutomationFocusChangedEventArgs e) { Interlocked.Increment(ref epoch); };
 public static void Start() { Automation.AddAutomationFocusChangedEventHandler(focus); }
 [DllImport("user32.dll")] static extern IntPtr OpenInputDesktop(uint f,bool inherit,uint access);
 [DllImport("user32.dll")] static extern bool CloseDesktop(IntPtr d);
 [DllImport("user32.dll")] static extern bool SwitchDesktop(IntPtr d);
 static bool Locked() { IntPtr d=OpenInputDesktop(0,false,0x0100); if(d==IntPtr.Zero)return true;try{return !SwitchDesktop(d);}finally{CloseDesktop(d);} }
 static int Points(string s) { int n=0;for(int i=0;i<s.Length;i++,n++)if(char.IsHighSurrogate(s[i])&&i+1<s.Length&&char.IsLowSurrogate(s[i+1]))i++;return n; }
 static int Units(string s,int points) { int i=0;for(int n=0;i<s.Length&&n<points;n++,i++)if(char.IsHighSurrogate(s[i])&&i+1<s.Length&&char.IsLowSurrogate(s[i+1]))i++;return i; }
 static string ID(AutomationElement e) { return string.Join(".",e.GetRuntimeId())+":"+Interlocked.Read(ref epoch); }
 public static Context Read(int window) {
  Context c=new Context();
  try {
   if(Locked()){c.Protected=true;return c;}
   AutomationElement e=AutomationElement.FocusedElement;
   if(e==null||!e.Current.HasKeyboardFocus)return c;
   if(e.Current.IsPassword){c.Protected=true;c.Password=true;c.Identity=ID(e);return c;}
   object p;
   if(!e.TryGetCurrentPattern(TextPattern.Pattern,out p))return c;
   TextPattern t=(TextPattern)p;
   if(!e.Current.IsEnabled || !object.Equals(t.DocumentRange.GetAttributeValue(TextPattern.IsReadOnlyAttribute),false))return c;
   TextPatternRange[] selection=t.GetSelection();if(selection.Length!=1)return c;
   TextPatternRange prefix=t.DocumentRange.Clone();
   prefix.MoveEndpointByRange(TextPatternRangeEndpoint.End,selection[0],TextPatternRangeEndpoint.Start);
   string before=prefix.GetText(1048577);if(before.Length>1048576)return c;
   string selected=selection[0].GetText(window*2+1);if(Points(selected)>window)return c;
   int start=Points(before),end=start+Points(selected);
   TextPatternRange view=selection[0].Clone();
   view.MoveEndpointByUnit(TextPatternRangeEndpoint.Start,TextUnit.Character,-window/2);
   view.MoveEndpointByUnit(TextPatternRangeEndpoint.End,TextUnit.Character,window);
   TextPatternRange offset=t.DocumentRange.Clone();
   offset.MoveEndpointByRange(TextPatternRangeEndpoint.End,view,TextPatternRangeEndpoint.Start);
   string pre=offset.GetText(1048577);if(pre.Length>1048576)return c;
   string value=view.GetText(window*2+1);
   int low=Points(pre);int take=Math.Min(window,Points(value));value=value.Substring(0,Units(value,take));
   if(end>low+take)return c;
   if(!e.Equals(AutomationElement.FocusedElement)||e.Current.IsPassword)return new Context();
   c.Identity=ID(e);c.Value=value;c.Start=start;c.End=end;c.WindowStart=low;c.Available=true;
   // Writable ValuePattern controls can be changed without injecting globally.
   object v;c.Editable=e.TryGetCurrentPattern(ValuePattern.Pattern,out v)&&!((ValuePattern)v).Current.IsReadOnly;
   if(c.Editable) {
    string writable=((ValuePattern)v).Current.Value;
    c.Editable=writable.Length<=1048576 && Points(writable)>=low+take && writable.Substring(Units(writable,low),Units(writable,low+take)-Units(writable,low))==value;
   }
   c.Multiline=e.Current.ControlType==ControlType.Document||value.IndexOf('\n')>=0;
   return c;
  } catch {return new Context { Protected=true };}
 }
 static TextPatternRange Range(TextPattern t,int start,int end) {
  TextPatternRange r=t.DocumentRange.Clone();r.MoveEndpointByRange(TextPatternRangeEndpoint.End,r,TextPatternRangeEndpoint.Start);
  // TextUnit.Character is provider-defined. Verify offsets after moving so a
  // grapheme-based provider can never silently edit the wrong UTF-16 range.
  r.MoveEndpointByUnit(TextPatternRangeEndpoint.Start,TextUnit.Character,start);
  r.MoveEndpointByRange(TextPatternRangeEndpoint.End,r,TextPatternRangeEndpoint.Start);
  r.MoveEndpointByUnit(TextPatternRangeEndpoint.End,TextUnit.Character,end-start);
  TextPatternRange prefix=t.DocumentRange.Clone();prefix.MoveEndpointByRange(TextPatternRangeEndpoint.End,r,TextPatternRangeEndpoint.Start);
  if(Points(prefix.GetText(1048577))!=start||Points(r.GetText(1048577))!=end-start)throw new Exception();return r;
 }
 public static bool Edit(int window,string identity,string value,int windowStart,int selStart,int selEnd,int start,int end,string text,bool selectionOnly) {
  try {
   Context now=Read(window);
   if(!now.Available||!now.Editable||now.Identity!=identity||now.Value!=value||now.WindowStart!=windowStart||now.Start!=selStart||now.End!=selEnd)return false;
   AutomationElement e=AutomationElement.FocusedElement; if(e.Current.IsPassword||ID(e)!=identity)return false;
   TextPattern t=(TextPattern)e.GetCurrentPattern(TextPattern.Pattern);
   if(selectionOnly){Range(t,start,end).Select();return true;}
   ValuePattern v=(ValuePattern)e.GetCurrentPattern(ValuePattern.Pattern);
   string full=v.Current.Value;if(full.Length>1048576||end>Points(full)||Points(full)<windowStart+Points(value))return false;
   if(full.Substring(Units(full,windowStart),Units(full,windowStart+Points(value))-Units(full,windowStart))!=value)return false;
   string next=full.Substring(0,Units(full,start))+text+full.Substring(Units(full,end));
   if(!e.Equals(AutomationElement.FocusedElement)||e.Current.IsPassword||Locked())return false;
   v.SetValue(next);
   int caret=start+Points(text);Range(t,caret,caret).Select();return true;
  }catch{return false;}
 }
}
'@
[OttomanText]::Start()
[Console]::WriteLine('ready')
while ($null -ne ($line = [Console]::ReadLine())) {
 try {
  $v = $line | ConvertFrom-Json
  if ($v.op -eq 'read') { $result = [OttomanText]::Read([int]$v.window) }
  else {
   $s=$v.expected; $e=$v.edit
   $result=@{OK=[OttomanText]::Edit([int]$v.window,$s.Identity,$s.Value,[int]$s.WindowStart,[int]$s.Start,[int]$s.End,[int]$e.Start,[int]$e.End,$e.Text,[bool]$e.SelectionOnly)}
  }
  [Console]::WriteLine(($result | ConvertTo-Json -Compress))
 } catch { [Console]::WriteLine('{}') }
}
