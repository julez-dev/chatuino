import{i as o,c as d,T as H,M as O,L as P,t as R}from"./index-BfGo2Bce.js";var U=R(`<div><h1 class="mb-8 text-3xl font-bold text-nord4"><span class=text-nord3>[</span><span class=text-nord8> Theme </span><span class=text-nord3>]</span></h1><p class="mb-6 text-nord4">You can configure the colors used by Chatuino with the theme.yaml file.</p><p class="mb-6 text-nord4">Your theme file is read from <code class="rounded bg-nord1 px-1.5 py-0.5 text-nord8">~/.config/chatuino/theme.yaml</code> (the config directory may differ depending on your OS). Create the file if it doesn't exist.</p><section class=mb-12><h2 class="mb-4 text-xl font-semibold text-nord4">Default Theme</h2><p class="mb-4 text-nord4">The default theme is inspired by the <a href=https://www.nordtheme.com/ target=_blank rel="noopener noreferrer"class="text-nord8 hover:text-nord7">Nord color scheme</a>:</p><pre class="overflow-x-auto rounded-lg border border-nord2 bg-nord1 p-4 text-sm"><code class=text-nord4># Emote provider colors
seven_tv_emote_color: "#88c0d0"
twitch_tv_emote_color: "#b48ead"
better_ttv_emote_color: "#bf616a"

# Input
input_prompt_color: "#88c0d0"

# Chat user colors
chat_streamer_color: "#d08770"
chat_vip_color: "#b48ead"
chat_sub_color: "#a3be8c"
chat_turbo_color: "#5e81ac"
chat_moderator_color: "#a3be8c"
chat_indicator_color: "#88c0d0"

# Alert colors
chat_sub_alert_color: "#b48ead"
chat_notice_alert_color: "#ebcb8b"
chat_clear_chat_color: "#d08770"
chat_error_color: "#bf616a"

# List/selection colors
list_selected_color: "#88c0d0"
list_label_color: "#81a1c1"
active_label_color: "#ebcb8b"

# Status
status_color: "#88c0d0"

# Splash screen
chatuino_splash_color: "#8fbcbb"
splash_highlight_color: "#d8dee9"

# Tab headers
tab_header_background_color: "#3b4252"
tab_header_active_background_color: "#2e3440"

# Borders
inspect_border_color: "#5e81ac"

# List styling
list_background_color: "#2e3440"
list_font_color: "#d8dee9"

# UI chrome
dimmed_text_color: "#4c566a"</code></pre></section><section class=mb-12><h2 class="mb-4 text-xl font-semibold text-nord4">Color Reference</h2><p class="mb-4 text-nord4">Here's a visual reference of the Nord color palette used by Chatuino:</p><div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-4"><div><h3 class="mb-2 text-sm font-medium text-nord4">Polar Night</h3><div class=space-y-2><div class="flex items-center gap-2"><div class="h-6 w-6 rounded"style=background-color:#2e3440></div><code class="text-xs text-nord4">#2e3440</code></div><div class="flex items-center gap-2"><div class="h-6 w-6 rounded"style=background-color:#3b4252></div><code class="text-xs text-nord4">#3b4252</code></div><div class="flex items-center gap-2"><div class="h-6 w-6 rounded"style=background-color:#434c5e></div><code class="text-xs text-nord4">#434c5e</code></div><div class="flex items-center gap-2"><div class="h-6 w-6 rounded"style=background-color:#4c566a></div><code class="text-xs text-nord4">#4c566a</code></div></div></div><div><h3 class="mb-2 text-sm font-medium text-nord4">Snow Storm</h3><div class=space-y-2><div class="flex items-center gap-2"><div class="h-6 w-6 rounded border border-nord2"style=background-color:#d8dee9></div><code class="text-xs text-nord4">#d8dee9</code></div><div class="flex items-center gap-2"><div class="h-6 w-6 rounded border border-nord2"style=background-color:#e5e9f0></div><code class="text-xs text-nord4">#e5e9f0</code></div><div class="flex items-center gap-2"><div class="h-6 w-6 rounded border border-nord2"style=background-color:#eceff4></div><code class="text-xs text-nord4">#eceff4</code></div></div></div><div><h3 class="mb-2 text-sm font-medium text-nord4">Frost</h3><div class=space-y-2><div class="flex items-center gap-2"><div class="h-6 w-6 rounded"style=background-color:#8fbcbb></div><code class="text-xs text-nord4">#8fbcbb</code></div><div class="flex items-center gap-2"><div class="h-6 w-6 rounded"style=background-color:#88c0d0></div><code class="text-xs text-nord4">#88c0d0</code></div><div class="flex items-center gap-2"><div class="h-6 w-6 rounded"style=background-color:#81a1c1></div><code class="text-xs text-nord4">#81a1c1</code></div><div class="flex items-center gap-2"><div class="h-6 w-6 rounded"style=background-color:#5e81ac></div><code class="text-xs text-nord4">#5e81ac</code></div></div></div><div><h3 class="mb-2 text-sm font-medium text-nord4">Aurora</h3><div class=space-y-2><div class="flex items-center gap-2"><div class="h-6 w-6 rounded"style=background-color:#bf616a></div><code class="text-xs text-nord4">#bf616a</code></div><div class="flex items-center gap-2"><div class="h-6 w-6 rounded"style=background-color:#d08770></div><code class="text-xs text-nord4">#d08770</code></div><div class="flex items-center gap-2"><div class="h-6 w-6 rounded"style=background-color:#ebcb8b></div><code class="text-xs text-nord4">#ebcb8b</code></div><div class="flex items-center gap-2"><div class="h-6 w-6 rounded"style=background-color:#a3be8c></div><code class="text-xs text-nord4">#a3be8c</code></div><div class="flex items-center gap-2"><div class="h-6 w-6 rounded"style=background-color:#b48ead></div><code class="text-xs text-nord4">#b48ead</code></div></div></div></div></section><section><h2 class="mb-4 text-xl font-semibold text-nord4">Custom Theme Example</h2><p class="mb-4 text-nord4">You can override any of the default colors. For example, to use a warmer color scheme:</p><pre class="overflow-x-auto rounded-lg border border-nord2 bg-nord1 p-4 text-sm"><code class=text-nord4># Custom warm theme
input_prompt_color: "#d08770"
list_selected_color: "#d08770"
status_color: "#d08770"
chat_indicator_color: "#d08770"`);function q(){return(()=>{var e=U(),t=e.firstChild,g=t.nextSibling,p=g.nextSibling,C=p.nextSibling,$=C.nextSibling,y=$.firstChild,S=y.nextSibling,w=S.nextSibling,s=w.firstChild,k=s.firstChild,T=k.nextSibling,l=T.firstChild;l.firstChild;var r=l.nextSibling;r.firstChild;var c=r.nextSibling;c.firstChild;var L=c.nextSibling;L.firstChild;var i=s.nextSibling,N=i.firstChild,Y=N.nextSibling,a=Y.firstChild;a.firstChild;var n=a.nextSibling;n.firstChild;var z=n.nextSibling;z.firstChild;var x=i.nextSibling,A=x.firstChild,E=A.nextSibling,b=E.firstChild;b.firstChild;var h=b.nextSibling;h.firstChild;var v=h.nextSibling;v.firstChild;var F=v.nextSibling;F.firstChild;var I=x.nextSibling,M=I.firstChild,B=M.nextSibling,_=B.firstChild;_.firstChild;var m=_.nextSibling;m.firstChild;var f=m.nextSibling;f.firstChild;var u=f.nextSibling;u.firstChild;var D=u.nextSibling;return D.firstChild,o(e,d(H,{children:"Theme - Chatuino"}),t),o(e,d(O,{name:"description",content:"Customize Chatuino colors with theme.yaml. Nord color scheme by default, fully customizable."}),t),o(e,d(P,{rel:"canonical",href:"https://chatuino.net/docs/theme"}),t),e})()}export{q as default};
