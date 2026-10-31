import { describe, expect, test } from "bun:test";
import { RemoteText, codePointOffset, utf16Offset, previousCharacterOffset, textPreview, textMirroringStatus } from "../src/remoteText";
import { diffText } from "../src/trackpadText";
import type { TrackpadMessage, TrackpadMessageTextContext } from "../src/api";
const context: TrackpadMessageTextContext = {
 type:"textcontext", fieldId:"focus-a", revision:1, value:"a😀b",selectionStart:2,selectionEnd:2,
 windowStart:0, available:true, editable:true, multiline:true,protected:false,password:false,
};
function setup() {
 const sent: TrackpadMessage[]=[];const model=new RemoteText(v=>sent.push(v));model.receive(context);return {model,sent};
}
describe("native text synchronization",()=>{
 test("serializes rapid edits using the acknowledged revision",()=>{
  const {model,sent}=setup();model.replace(2,2,"x");model.replace(3,3,"y");expect(sent.length).toBe(1);
  model.receive({...context,value:"a😀xb",selectionStart:3,selectionEnd:3,revision:2,requestId:1});
  expect(sent.length).toBe(2);expect(sent[1]).toMatchObject({baseRevision:2,start:3,text:"y"});
  expect(model.view?.value).toBe("a😀xyb");
  model.receive({...context,value:"a😀xyb",selectionStart:4,selectionEnd:4,revision:3,requestId:2});expect(model.view?.revision).toBe(3);
 });
 test("keeps a fast typing burst through desktop notifications before each acknowledgment",()=>{
  const {model,sent}=setup();
  const burst=Array.from(" really fast 😀 typing");
  let caret=2;
  for(const char of burst) {model.replace(caret,caret,char);caret++;}
  const optimistic=model.view!.value;
  let value=context.value;
  for(let i=0;i<burst.length;i++) {
   const chars=Array.from(value); chars.splice(2+i,0,burst[i]); value=chars.join("");
   // A newer poll result used to discard the remaining keystrokes here.
   const echo={...context,value,selectionStart:3+i,selectionEnd:3+i,revision:2+i};
   model.receive(echo);
   expect(model.view?.value).toBe(optimistic);
   model.receive({...echo,requestId:i+1});
   expect(model.view?.value).toBe(optimistic);
  }
  expect(sent.length).toBe(burst.length);
  expect(model.context?.value).toBe(optimistic);
 });
 test("preserves every queued letter while Chrome keeps appending a selected suggestion",()=>{
  const {model,sent}=setup();
  model.receive({...context,value:"",selectionStart:0,selectionEnd:0});
  for (const [i,char] of Array.from("going").entries()) model.replace(i,i,char);
  let prefix="";
  for (const [i,char] of Array.from("going").entries()) {
   const edit=sent[i];
   expect(edit.type).toBe("textedit");
   if(edit.type!=="textedit") throw Error("Expected text edit");
   expect(edit.start).toBe(i);
   expect(edit.end).toBe(i===0?0:i+6);
   prefix+=char;
   const next={...context,value:prefix+"oogle!",selectionStart:i+1,selectionEnd:i+7,revision:i+2,requestId:edit.requestId};
   // A poll can arrive before the matching acknowledgment.
   model.receive({...next,requestId:undefined});
   model.receive(next);
   if(i<4) expect(model.view?.value).toBe("going");
  }
  expect(model.view?.value).toBe("goingoogle!");
  expect(sent.length).toBe(5);
 });
 test("retries an unapplied letter when a late completion advances the revision",()=>{
  const {model,sent}=setup();
  model.receive({...context,value:"g",selectionStart:1,selectionEnd:1});
  model.replace(1,1,"o"); model.replace(2,2,"i"); model.replace(3,3,"n"); model.replace(4,4,"g");
  model.receive({...context,value:"google.com",selectionStart:1,selectionEnd:10,revision:2,requestId:1,rejected:true});
  expect(sent[1]).toMatchObject({type:"textedit",start:1,end:10,text:"o",baseRevision:2,requestId:5});
  expect(model.view?.value).toBe("going");
  model.receive({...context,value:"go",selectionStart:2,selectionEnd:2,revision:3,requestId:5});
  expect(sent[2]).toMatchObject({start:2,end:2,text:"i",baseRevision:3});
 });
 test("retries replacement of a selected suggestion when only its suffix changes",()=>{
  const {model,sent}=setup();
  model.receive({...context,value:"google.com",selectionStart:1,selectionEnd:10});
  model.replace(1,10,"o"); model.replace(2,2,"i");
  model.receive({...context,value:"github.com",selectionStart:1,selectionEnd:10,revision:2,requestId:1,rejected:true});
  expect(sent[1]).toMatchObject({start:1,end:10,text:"o",baseRevision:2});
  expect(model.view?.value).toBe("goi");
  model.receive({...context,value:"go",selectionStart:2,selectionEnd:2,revision:3,requestId:3});
  expect(sent[2]).toMatchObject({start:2,end:2,text:"i",baseRevision:3});
 });
 test("does not retry an unchanged failed write or a different typed prefix",()=>{
  for (const [value,revision] of [["google.com",1],["other.com",2]] as const) {
   const {model,sent}=setup();
   model.receive({...context,value:"google.com",selectionStart:1,selectionEnd:10});
   model.replace(1,10,"o"); model.replace(2,2,"i");
   model.receive({...context,value,selectionStart:value==="other.com"?2:1,selectionEnd:value.length,revision,requestId:1,rejected:true});
   expect(sent.length).toBe(1);
   expect(model.view?.value).toBe(value);
  }
 });
 test("does not treat an external replacement as inline autocomplete",()=>{
  const {model,sent}=setup();
  model.receive({...context,value:"g",selectionStart:1,selectionEnd:1});
  model.replace(1,1,"o"); model.replace(2,2,"i");
  model.receive({...context,value:"other",selectionStart:2,selectionEnd:5,revision:2,requestId:1});
  expect(model.view?.value).toBe("other"); expect(sent.length).toBe(1);
 });
 test("privacy changes immediately clear a pending typing burst",()=>{
  const {model}=setup();model.replace(2,2,"x");model.replace(3,3,"y");
  model.receive({...context,available:false,editable:false,protected:true,value:""});
  expect(model.view?.value).toBe("");expect(model.view?.protected).toBe(true);
 });
 test("discards pending edits when native focus changes",()=>{
  const {model,sent}=setup();model.replace(2,2,"x");model.replace(3,3,"y");
  model.receive({...context,fieldId:"focus-b",value:"other",revision:3});expect(model.view?.value).toBe("other");expect(sent.length).toBe(1);
 });
 test("resynchronizes after a rejected stale edit",()=>{
  const {model,sent}=setup();model.replace(2,2,"x");model.replace(3,3,"y");
  model.receive({...context,value:"local edit",revision:3,requestId:1,rejected:true});expect(model.view?.value).toBe("local edit");expect(sent.length).toBe(1);
 });
 test("replaces selections using absolute code-point offsets",()=>{
  const {model,sent}=setup();model.receive({...context,windowStart:100,selectionStart:101,selectionEnd:102});
  model.replace(101,102,"👍");expect(model.view?.value).toBe("a👍b");expect(sent[0]).toMatchObject({start:101,end:102});
 });
 test("does not edit a protected or unavailable context",()=>{
  const {model,sent}=setup();model.receive({...context,value:"",available:false,protected:true});expect(model.replace(0,0,"secret")).toBe(false);expect(sent.length).toBe(0);
  model.clear();expect(model.view).toBeNull();expect(model.context).toBeNull();
 });
 test("does not split a shared emoji surrogate pair",()=>{
  expect(diffText("a😀b","a😁b")).toEqual({start:1,removed:"😀",inserted:"😁"});
  expect(codePointOffset("a😀b",3)).toBe(2);expect(utf16Offset("a😀b",2)).toBe(3);
 });
 test("moves a selection without changing text",()=>{
  const {model,sent}=setup();model.replace(0,2,"",true);expect(model.view?.value).toBe("a😀b");expect(model.view?.selectionEnd).toBe(2);expect(sent[0]).toMatchObject({selectionOnly:true});
 });
});

test("backspace keeps joined emoji and combining marks intact", () => {
 expect(previousCharacterOffset("a👨‍👩‍👧‍👦", 8)).toBe(1);
 expect(previousCharacterOffset("ae\u0301", 3)).toBe(1);
 expect(previousCharacterOffset("مرحبا", 5)).toBe(4);
});

describe("password preview masking", () => {
 test("uses only the explicit character count and ignores raw text and selections", () => {
  const password = {...context, protected:true, password:true, value:"must never appear", selectionStart:10, maskedLength:6};
  expect(textPreview(password, "also secret")).toBe("••••••");
  expect(textPreview({...password, maskedLength:0}, "secret")).toBe("");
 });
 test("shows a fixed mask when protected length is unavailable", () => {
  expect(textPreview({...context, password:true, protected:true, maskedLength:undefined}, "secret")).toBe("••••••••");
 });
 test("bounds malformed lengths and keeps locked or unavailable sessions blank", () => {
  expect(textPreview({...context,password:true,maskedLength:99999}, "secret").length).toBe(4096);
  expect(textPreview({...context,password:true,maskedLength:-1}, "secret")).toBe("••••••••");
  expect(textPreview({...context,password:true,maskedLength:NaN}, "secret")).toBe("••••••••");
  expect(textPreview({...context,protected:true,available:false}, "secret")).toBe("");
  expect(textPreview(null, "ordinary draft")).toBe("ordinary draft");
 });
 test("keeps password typing on the keystroke path", () => {
  const {model,sent}=setup();
  model.receive({...context, protected:true, password:true, maskedLength:5});
  expect(model.replace(0,1,"x")).toBe(false);
  expect(sent.length).toBe(0);
 });
});

test("native mirroring status distinguishes a real context from fallback typing", () => {
 expect(textMirroringStatus(false, context)).toBe("Desktop text: disconnected");
 expect(textMirroringStatus(true, null)).toBe("Desktop text: waiting for the agent");
 expect(textMirroringStatus(true, context)).toBe("Desktop text: connected");
 expect(textMirroringStatus(true, {...context, available:false})).toBe("Desktop text: no accessible text field detected");
 expect(textMirroringStatus(true, {...context, protected:true, available:false, value:"sensitive"})).toBe("Desktop text: access unavailable or screen locked");
 expect(textMirroringStatus(true, {...context, protected:true, password:true})).toBe("Desktop text: password field (contents hidden)");
});
