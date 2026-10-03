package overlay

import "github.com/wailsapp/wails/v3/pkg/application"

const blankPage = "about:blank"

// Wails registers WebviewWindowOptions.JS as a document-created script only for
// a window whose first document comes from HTML, so the browser starts on this
// empty page instead of about:blank.
const browserHTML = `<!doctype html><meta charset="utf-8"><title>Typhon</title>`

// browserGuard runs in every document of the overlay browser before the page's
// own scripts. Wails delivers every launcher event to every window by
// evaluating window._wails.dispatchWailsEvent(...) in its page, so the hook is
// pinned to an empty object the page cannot replace or reach through
// Object.prototype. Wails also leaves new-window requests to WebView2, which
// opens them as top-level windows outside the overlay and its permission
// policy, so window.open and links or forms aimed at a new window navigate
// this one instead. Elements are recognised by tag name because instanceof
// fails for SVG links and for nodes made in another frame, and shadow roots
// are created open because a click inside a closed one never shows the link.
const browserGuard = `Object.defineProperty(window,'_wails',{value:Object.freeze(Object.create(null)),writable:false,configurable:false,enumerable:false});
(function(){
'use strict';
var doc=document,Url=URL,nativeOpen=window.open,nativeAttach=Element.prototype.attachShadow,activation=navigator.userActivation;
var getAttr=Element.prototype.getAttribute,getAttrNS=Element.prototype.getAttributeNS,setAttr=Element.prototype.setAttribute,hasAttr=Element.prototype.hasAttribute;
var nodeType=Object.getOwnPropertyDescriptor(Node.prototype,'nodeType').get,localName=Object.getOwnPropertyDescriptor(Element.prototype,'localName').get;
var listen=EventTarget.prototype.addEventListener;
var formAction=Object.getOwnPropertyDescriptor(HTMLFormElement.prototype,'action').get;
var nativeSubmit=HTMLFormElement.prototype.submit;
function tag(n){try{return nodeType.call(n)===1?localName.call(n):'';}catch(e){return '';}}
function web(raw){
var u;
try{u=new Url(String(raw),doc.baseURI);}catch(e){return '';}
return u.protocol==='http:'||u.protocol==='https:'?u.href:'';
}
function here(target){
var t=target.toLowerCase();
if(t===''||t==='_blank')return false;
if(t==='_self'||t==='_parent'||t==='_top'||target===window.name)return true;
var named=doc.getElementsByName(target);
for(var i=0;i<named.length;i++){var k=tag(named[i]);if(k==='iframe'||k==='frame')return true;}
return false;
}
function go(href){try{window.top.location.href=href;}catch(e){}}
function base(){var b=doc.querySelector('base[target]');return b?getAttr.call(b,'target'):'';}
function open(url,target){
var name=target===undefined?'':String(target);
if(here(name))return nativeOpen.apply(window,arguments);
if(activation&&!activation.isActive)return null;
var href=url===undefined||url===''?'':web(url);
if(href)go(href);
return null;
}
Object.defineProperty(window,'open',{get:function(){return open;},set:function(){},enumerable:true,configurable:false});
function linkOf(event){
var path=event.composedPath();
for(var i=0;i<path.length;i++){var t=tag(path[i]);if(t==='a'||t==='area')return path[i];}
return null;
}
function hrefOf(link){
var raw=getAttr.call(link,'href');
return raw===null?getAttrNS.call(link,'http://www.w3.org/1999/xlink','href'):raw;
}
function onLink(event){
if(event.type==='auxclick'&&event.button!==1)return;
var link=linkOf(event);
if(!link||hasAttr.call(link,'download'))return;
var raw=hrefOf(link);
if(raw===null)return;
var fresh=event.button===1||event.ctrlKey||event.shiftKey||event.metaKey;
if(!fresh&&here(getAttr.call(link,'target')||base()||'_self'))return;
var href=web(raw);
if(!href){event.preventDefault();return;}
if(fresh){event.preventDefault();go(href);return;}
setAttr.call(link,'target','_top');
}
function aim(form,submitter){
var own=submitter&&hasAttr.call(submitter,'formtarget');
if(here((own?getAttr.call(submitter,'formtarget'):getAttr.call(form,'target'))||base()||'_self'))return true;
if(!web(submitter&&hasAttr.call(submitter,'formaction')?submitter.formAction:formAction.call(form)))return false;
if(own)setAttr.call(submitter,'formtarget','_top');
setAttr.call(form,'target','_top');
return true;
}
function onSubmit(event){
var form=event.target;
if(tag(form)==='form'&&!aim(form,event.submitter))event.preventDefault();
}
function attach(init){
if(init===null||typeof init!=='object')return nativeAttach.call(this,init);
var opts={};
for(var k in init)opts[k]=init[k];
opts.mode='open';
var root=nativeAttach.call(this,opts);
listen.call(root,'submit',onSubmit,true);
return root;
}
Object.defineProperty(Element.prototype,'attachShadow',{get:function(){return attach;},set:function(){},enumerable:true,configurable:false});
window.addEventListener('click',onLink,true);
window.addEventListener('auxclick',onLink,true);
window.addEventListener('submit',onSubmit,true);
HTMLFormElement.prototype.submit=function(){if(aim(this,null))nativeSubmit.call(this);};
})();`

const exitFullscreenJS = `if(document.fullscreenElement){document.exitFullscreen().catch(function(){});}`

func browserOptions(onEscape func()) application.WebviewWindowOptions {
	deny := map[application.PermissionType]application.Permission{
		application.PermissionMicrophone:    application.PermissionDeny,
		application.PermissionCamera:        application.PermissionDeny,
		application.PermissionGeolocation:   application.PermissionDeny,
		application.PermissionNotifications: application.PermissionDeny,
		application.PermissionClipboardRead: application.PermissionDeny,
	}
	return application.WebviewWindowOptions{
		Name: BrowserWindowName, Title: "Typhon", Width: 1280, Height: 720,
		HTML: browserHTML, JS: browserGuard,
		Hidden: true, Frameless: true, DisableResize: true, AlwaysOnTop: true,
		Permissions: deny,
		KeyBindings: map[string]func(application.Window){"escape": escapeBinding(onEscape)},
		Windows: application.WindowsWindow{
			HiddenOnTaskbar:         true,
			GeneralAutofillEnabled:  false,
			PasswordAutosaveEnabled: false,
		},
	}
}

// A bound key never reaches the page, so Esc has to leave a page's fullscreen
// itself before it can mean "back to the game".
func escapeBinding(onEscape func()) func(application.Window) {
	return func(w application.Window) {
		if w.IsFullscreen() {
			w.ExecJS(exitFullscreenJS)
			return
		}
		onEscape()
	}
}
