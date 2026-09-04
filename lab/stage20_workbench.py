#!/usr/bin/env python3
"""A real, local note workspace; its DOM is used only by Lab for ground truth."""
import argparse
import json
import pathlib
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

HTML = r'''<!doctype html><html lang="zh"><meta charset="utf-8"><title>Hominal20 工作室</title>
<style>*{box-sizing:border-box}body{font:18px system-ui;background:#eef2f6;color:#152739;margin:0}header{background:#183149;color:white;padding:20px 30px;display:flex;justify-content:space-between}main{display:grid;grid-template-columns:230px 1fr;gap:26px;margin:28px}aside,article{background:white;border-radius:14px;padding:24px}button,input,textarea{font:inherit}button{border:0;border-radius:8px;padding:12px 18px;margin:7px 6px 7px 0;cursor:pointer;background:#e2eaf0;color:#19344e}button.primary{background:#167466;color:white}label{display:block;margin:20px 0 8px}input,textarea{border:1px solid #aabcc9;border-radius:8px;width:100%;padding:12px}textarea{height:200px}#status{padding:12px;color:#146352;min-height:50px}#library{padding:18px;background:#f4f8fb;line-height:1.8}dialog{border:0;border-radius:12px;padding:30px;max-width:550px}body.alt main{grid-template-columns:1fr 230px}body.alt aside{grid-column:2;grid-row:1}body.alt article{grid-column:1;grid-row:1}body.alt .actions{display:flex;flex-direction:row-reverse;justify-content:start}body.alt textarea{height:250px}body.dark{background:#dfe7ee}#probe{display:none}.spacer{height:200px}</style>
<header><strong>Hominal20 · 视觉工作室</strong><span>本地笔记与阅读空间</span></header>
<main><aside><h3>工作区</h3><button id="new" onclick="newNote()">新建笔记</button><button id="libraryButton" onclick="toggleLibrary()">阅读资料</button><button id="layout" onclick="layout()">切换布局</button><button id="helpButton" onclick="help.showModal()">使用说明</button><hr><div id="savedList"></div></aside>
<article><h2 id="heading">一页空白，留给你的想法</h2><label for="title">笔记标题</label><input id="title" placeholder="为这页内容起个名字"><label for="body">笔记内容</label><textarea id="body" placeholder="可以记录见闻、写下问题、整理想法……"></textarea><div class="actions"><button class="primary" id="save" onclick="save()">保存笔记</button><button id="clear" onclick="document.querySelector('#body').value=''">清空正文</button><button id="preview" onclick="preview()">预览笔记</button></div><div id="status" role="status">内容保存在这台设备的当前工作区。</div><section id="library" hidden><h3>经验与变化</h3><p>学习一个操作，可以记住按钮的位置，也可以理解它的作用。窗口移动以后，位置可能失效，而按钮的文字、周围关系和操作结果仍可以帮助重新找到它。</p><p>一次尝试之后，观察实际变化，才能判断自己的预期是否得到支持。相同方法在不同情境里可能需要调整。</p><p>这是一段可讨论的材料，不是要求你接受的结论。你可以记录赞同、疑问或自己的例子。</p></section><div class="spacer"></div><button id="bottom" onclick="document.querySelector('#status').textContent='已经抵达页面下方。'">页面下方的按钮</button></article></main>
<dialog id="help"><h2>使用说明</h2><p>这里可以写笔记、保存后重新打开、阅读材料和切换布局。保存会在当前工作区创建真实文件。布局变化不会改变笔记内容。</p><button onclick="help.close()" id="closeHelp">关闭说明</button></dialog>
<script>const qs=s=>document.querySelector(s);let current='';let help=qs('#help');let saves=0;function newNote(){current='';qs('#title').value='';qs('#body').value='';qs('#status').textContent='已新建空白笔记。'}function layout(){document.body.classList.toggle('alt');qs('#status').textContent='布局已切换，笔记内容保留。'}function toggleLibrary(){qs('#library').hidden=false;qs('#status').textContent='资料已打开：经验与变化。';qs('#library').scrollIntoView({block:'start'})}function preview(){qs('#status').textContent='预览：'+qs('#title').value+' — '+qs('#body').value}async function save(){let v={title:qs('#title').value,body:qs('#body').value,id:current};let r=await fetch('/notes',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(v)});let j=await r.json();if(!r.ok||!j.saved){qs('#status').textContent='保存失败，正文仍保留，尚未写入文件。';return}current=j.id;qs('#status').textContent='已保存：'+v.title;await list()}async function list(){let items=await(await fetch('/notes')).json();qs('#savedList').replaceChildren();for(let item of items){let b=document.createElement('button');b.textContent=item.title||'无标题';b.onclick=()=>{current=item.id;qs('#title').value=item.title;qs('#body').value=item.body;qs('#status').textContent='已打开保存的笔记。'};qs('#savedList').append(b)}}let params=new URLSearchParams(location.search);if(params.get('layout')==='1')document.body.classList.add('alt');if(params.has('compact'))document.querySelector('main').style.margin='12px';list();</script></html>'''


def main():
    ap=argparse.ArgumentParser();ap.add_argument('--workspace',required=True);args=ap.parse_args()
    root=pathlib.Path(args.workspace).resolve();root.mkdir(parents=True,exist_ok=True)
    class Handler(BaseHTTPRequestHandler):
        def log_message(self,*a): pass
        def reply(self,code,data,typ='application/json'):
            raw=data.encode() if isinstance(data,str) else json.dumps(data,ensure_ascii=False).encode()
            self.send_response(code);self.send_header('Content-Type',typ+'; charset=utf-8');self.send_header('Content-Length',str(len(raw)));self.end_headers();self.wfile.write(raw)
        def do_GET(self):
            if self.path=='/notes': return self.reply(200,[json.loads(p.read_text()) for p in sorted(root.glob('note-*.json'))])
            self.reply(200,HTML,'text/html')
        def do_POST(self):
            if self.path!='/notes': return self.reply(404,{})
            n=int(self.headers.get('Content-Length','0'))
            if not 0<n<40000:return self.reply(400,{})
            try:
                failure=root/'.lab-fail-save-once'
                if failure.exists():
                    failure.unlink();return self.reply(503,{'error':'temporary local storage unavailability (Lab fixture)'})
                import re,uuid
                v=json.loads(self.rfile.read(n));ident=v.get('id') or 'note-'+uuid.uuid4().hex
                if not re.fullmatch('note-[0-9a-f]{32}',ident):raise ValueError('invalid ID')
                title=str(v['title'])[:300];body=str(v['body'])[:16000]
                if not title.strip() and not body.strip():return self.reply(422,{'error':'empty note'})
                record={'id':ident,'title':title,'body':body,'saved_at':time.time()}
                p=root/(ident+'.json');tmp=p.with_suffix('.tmp');tmp.write_text(json.dumps(record,ensure_ascii=False,indent=2));tmp.replace(p)
                self.reply(200,{'id':ident,'saved':True})
            except Exception:self.reply(400,{'error':'invalid note'})
    ThreadingHTTPServer(('127.0.0.1',8760),Handler).serve_forever()


if __name__=='__main__':main()
