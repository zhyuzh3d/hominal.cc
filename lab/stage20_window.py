#!/usr/bin/python3
"""Lab-only KWin geometry truth. Browser-reported screenX is unreliable on Wayland."""
import argparse,json,pathlib,uuid
import dbus,dbus.service
from dbus.mainloop.glib import DBusGMainLoop
from gi.repository import GLib

def main():
    p=argparse.ArgumentParser();p.add_argument('--rect');p.add_argument('--minimized',choices=['true','false']);a=p.parse_args()
    DBusGMainLoop(set_as_default=True);bus=dbus.SessionBus();name=dbus.service.BusName('org.hominal20.Lab',bus)
    loop=GLib.MainLoop();reply=[]
    class Receiver(dbus.service.Object):
        @dbus.service.method('org.hominal20.Lab',in_signature='s',out_signature='')
        def Report(self,text):reply.append(json.loads(str(text)));loop.quit()
    receiver=Receiver(name,'/Lab')
    rect=json.loads(a.rect) if a.rect else None
    if rect and (len(rect)!=4 or not all(isinstance(v,int) for v in rect)):raise ValueError('invalid rectangle')
    mutation=''
    if rect:mutation='w.fullScreen=false;w.setMaximize(false,false);w.frameGeometry='+json.dumps(dict(zip(['x','y','width','height'],rect)))+';'
    if a.minimized:mutation+='w.minimized='+a.minimized+';'+('workspace.activeWindow=w;' if a.minimized=='false' else '')
    script='''for (const w of workspace.windowList()) {if(w.caption.indexOf('Hominal20 工作室')>=0){%s const r=w.frameGeometry;callDBus('org.hominal20.Lab','/Lab','org.hominal20.Lab','Report',JSON.stringify({id:String(w.internalId),minimized:w.minimized,frame:[r.x,r.y,r.width,r.height]}));break;}}'''%mutation
    root=pathlib.Path.home()/'.local/share/hominal20/tools';file=root/'kwin-lab.js';file.write_text(script)
    iface=dbus.Interface(bus.get_object('org.kde.KWin','/Scripting'),'org.kde.kwin.Scripting');plugin='hominal20-lab-'+uuid.uuid4().hex
    try:
        ident=bus.call_blocking('org.kde.KWin','/Scripting','org.kde.kwin.Scripting','loadScript','ss',(str(file),plugin))
        iface.start()
        GLib.timeout_add_seconds(4,lambda:(loop.quit(),False)[1]);loop.run()
        if not reply:raise RuntimeError('KWin did not report the isolated workbench window')
        print(json.dumps(reply[0]))
    finally:iface.unloadScript(plugin)

if __name__=='__main__':main()
