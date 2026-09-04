#!/usr/bin/python3
"""An old sample's deadline cannot stop a later individual sharing the service name."""
import argparse,json,shlex,subprocess

def main():
    p=argparse.ArgumentParser();p.add_argument('--instance',required=True);p.add_argument('--unit',default='hominal20-life.service');a=p.parse_args()
    if not a.unit.startswith('hominal20-'):raise ValueError('deadline may only operate Stage20 units')
    raw=subprocess.check_output(['systemctl','--user','show',a.unit,'--property=Environment','--value'],text=True)
    env=dict(v.split('=',1) for v in shlex.split(raw) if '=' in v)
    matches=env.get('HOMINAL_INSTANCE_ID')==a.instance
    if matches:subprocess.run(['systemctl','--user','stop',a.unit],check=True,timeout=50)
    print(json.dumps({'instance':a.instance,'identity_matched':matches,'stopped':matches}))

if __name__=='__main__':main()
