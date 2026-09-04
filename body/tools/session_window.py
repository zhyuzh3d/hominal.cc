#!/usr/bin/python3
"""Read the compositor's actual active window; no window activation or movement."""
import json,os,pathlib,uuid
import dbus,dbus.service
from dbus.mainloop.glib import DBusGMainLoop
from gi.repository import GLib

def main():
    DBusGMainLoop(set_as_default=True);bus=dbus.SessionBus()
    name=dbus.service.BusName('org.hominal20.FocusProbe',bus);loop=GLib.MainLoop();reply=[]
    class Receiver(dbus.service.Object):
        @dbus.service.method('org.hominal20.FocusProbe',in_signature='s',out_signature='')
        def Report(self,text):reply.append(json.loads(str(text)));loop.quit()
    receiver=Receiver(name,'/Focus')
    key='hominal20-focus-'+uuid.uuid4().hex
    path=pathlib.Path(os.environ['HOMINAL20_ROOT'])/'services'/(key+'.js')
    path.write_text("const w=workspace.activeWindow;callDBus('org.hominal20.FocusProbe','/Focus','org.hominal20.FocusProbe','Report',JSON.stringify(w?{window_id:String(w.internalId),caption:w.caption,minimized:w.minimized,active:w.active,frame:[w.frameGeometry.x,w.frameGeometry.y,w.frameGeometry.width,w.frameGeometry.height]}:{}));")
    iface=dbus.Interface(bus.get_object('org.kde.KWin','/Scripting'),'org.kde.kwin.Scripting')
    try:
        ident=bus.call_blocking('org.kde.KWin','/Scripting','org.kde.kwin.Scripting','loadScript','ss',(str(path),key))
        script=dbus.Interface(bus.get_object('org.kde.KWin','/Scripting/Script'+str(ident)),'org.kde.kwin.Script')
        script.run()
        GLib.timeout_add(1000,lambda:(loop.quit(),False)[1]);loop.run()
        if not reply:raise RuntimeError('active window cannot be confirmed')
        reply[0]['showing_desktop']=bool(bus.call_blocking('org.kde.KWin','/KWin','org.freedesktop.DBus.Properties','Get','ss',('org.kde.KWin','showingDesktop')))
        print(json.dumps(reply[0]))
    finally:
        iface.unloadScript(key);path.unlink(missing_ok=True)

if __name__=='__main__':main()
