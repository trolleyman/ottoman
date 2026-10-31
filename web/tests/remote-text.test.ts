import { describe, expect, test } from "bun:test";
import { RemoteText, codePointOffset, utf16Offset, previousCharacterOffset } from "../src/remoteText";
import { diffText } from "../src/trackpadText";
import type { TrackpadMessage, TrackpadMessageTextContext } from "../src/api";
const context: TrackpadMessageTextContext = {
 type:"textcontext", fieldId:"focus-a", revision:1, value:"a😀b",selectionStart:2,selectionEnd:2,
 windowStart:0, available:true, editable:true, multiline:true,protected:false,
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
