#!/usr/bin/env python3
"""Session-user input broker. Uses the user's existing uinput ACL; never elevates."""
import json
import os
import pathlib
import secrets
import subprocess
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

KEYS = {'Enter':[28], 'Escape':[1], 'Tab':[15], 'Backspace':[14],
        'Ctrl+A':[29,30], 'Ctrl+S':[29,31], 'Ctrl+V':[29,47]}


def main():
    from evdev import UInput, AbsInfo, ecodes as e
    root = pathlib.Path(os.environ['HOMINAL20_ROOT'])
    os.umask(0o077)
    token = secrets.token_hex(32)
    (root/'services/input.cap').write_text(token)
    touch = UInput({e.EV_KEY:[e.BTN_TOUCH], e.EV_ABS:[
        (e.ABS_X,AbsInfo(0,0,65535,0,0,1)),(e.ABS_Y,AbsInfo(0,0,65535,0,0,1))]},
        name='Hominal20 visual touch', input_props=[e.INPUT_PROP_DIRECT])
    keyboard = UInput({e.EV_KEY: sorted(set(k for v in KEYS.values() for k in v)),
                       e.EV_REL:[e.REL_WHEEL]},name='Hominal20 keyboard')
    gate=threading.Lock()

    def key(name):
        codes=KEYS[name]
        try:
            for code in codes: keyboard.write(e.EV_KEY,code,1)
            keyboard.syn(); time.sleep(.04)
        finally:
            for code in reversed(codes): keyboard.write(e.EV_KEY,code,0)
            keyboard.syn()

    class Handler(BaseHTTPRequestHandler):
        def log_message(self,*a): pass
        def reply(self,code,obj):
            raw=json.dumps(obj).encode()
            try:
                self.send_response(code); self.send_header('Content-Type','application/json')
                self.send_header('Content-Length',str(len(raw))); self.end_headers(); self.wfile.write(raw)
            except (BrokenPipeError,ConnectionResetError): pass
        def do_POST(self):
            if not secrets.compare_digest(self.headers.get('X-Hominal-Capability',''),token):
                return self.reply(403,{'error':'invalid local capability'})
            if self.path!='/input': return self.reply(404,{'error':'unknown route'})
            n=int(self.headers.get('Content-Length','0'))
            if not 0<n<40000: return self.reply(400,{'error':'invalid size'})
            delivered=False
            try:
                req=json.loads(self.rfile.read(n)); p=req['payload']; kind=req['kind']
                with gate:
                    if time.time()>=req['deadline']: raise ValueError('expired before input')
                    locked=subprocess.check_output(['/usr/lib64/qt6/bin/qdbus','org.freedesktop.ScreenSaver',
                           '/ScreenSaver','org.freedesktop.ScreenSaver.GetActive'],text=True,timeout=2).strip()
                    if locked!='false': raise ValueError('session locked')
                    scope=json.loads((root/'services/input-scope.json').read_text())
                    focus=json.loads(subprocess.check_output(['/usr/bin/python3',str(pathlib.Path(__file__).with_name('session_window.py'))],text=True,timeout=2))
                    if (not scope.get('window_id') or focus.get('window_id')!=scope['window_id']
                            or focus.get('minimized') is not False or focus.get('active') is not True
                            or focus.get('showing_desktop') is not False):
                        raise ValueError('foreground window is outside the current experiment surface; no input delivered')
                    if kind=='tap':
                        x,y=p['x'],p['y']
                        if not (type(x) in (float,int) and type(y) in (float,int) and 0<=x<1 and 0<=y<1):
                            raise ValueError('invalid normalized point')
                        width,height=p['display_size'];fx,fy,fw,fh=focus['frame']
                        if not (fx<=x*width<fx+fw and fy<=y*height<fy+fh):raise ValueError('point is outside experiment window; no input delivered')
                        delivered=True
                        touch.write(e.EV_ABS,e.ABS_X,round(x*65535));touch.write(e.EV_ABS,e.ABS_Y,round(y*65535))
                        try:
                            touch.write(e.EV_KEY,e.BTN_TOUCH,1);touch.syn();time.sleep(.07)
                        finally: touch.write(e.EV_KEY,e.BTN_TOUCH,0);touch.syn()
                    elif kind=='key':
                        if p['key'] not in KEYS:raise ValueError('unsupported key')
                        delivered=True;key(p['key'])
                    elif kind=='scroll':
                        value=p['amount']
                        if type(value)!=int or not -8<=value<=8: raise ValueError('invalid scroll')
                        delivered=True;keyboard.write(e.EV_REL,e.REL_WHEEL,value);keyboard.syn()
                    elif kind=='text':
                        value=p['text']
                        if not isinstance(value,str) or len(value)>8000: raise ValueError('invalid text')
                        subprocess.run(['wl-copy','--type','text/plain;charset=utf-8'],input=value,text=True,check=True,timeout=2)
                        delivered=True;key('Ctrl+V')
                    else: raise ValueError('unsupported input')
                    self.reply(200,{'delivered':True,'kind':kind,'at':time.time(),'semantic_success':'not_decided'})
            except Exception as exc: self.reply(422,{'error':str(exc)[:200],'input_attempted':delivered})
        def do_GET(self):
            self.reply(200 if self.path=='/health' else 404,{'ready':True,'busy':gate.locked()})
    # Reading /dev/input is not required for writing this user's virtual device.
    print(json.dumps({'ready':True,'device':'Hominal20 visual touch'}),flush=True)
    try: ThreadingHTTPServer(('127.0.0.1',8766),Handler).serve_forever()
    finally: touch.close();keyboard.close()


if __name__=='__main__': main()
