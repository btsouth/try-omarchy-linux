import subprocess
code=r'''
import multiprocessing as mp,time,hashlib,os,pathlib
sentinel=pathlib.Path('/home/omarchy/Documents/astra-release-sentinel.txt')
before=hashlib.sha256(sentinel.read_bytes()).hexdigest()
def load():
 data=bytearray(512*1024*1024)
 for i in range(0,len(data),4096): data[i]=i%251
 end=time.monotonic()+90
 while time.monotonic()<end: hashlib.sha256(data).digest()
p=mp.get_context("fork").Process(target=load);p.start()
path=pathlib.Path('/home/omarchy/.cache/astra-load-fixture')
data=os.urandom(16*1024*1024); expected=hashlib.sha256(data*4).hexdigest()
for i in range(8):
 with path.open('wb') as f:
  for n in range(4): f.write(data)
  f.flush();os.fsync(f.fileno())
 assert hashlib.sha256(path.read_bytes()).hexdigest()==expected
 time.sleep(2)
p.join(timeout=100); assert p.exitcode==0
assert hashlib.sha256(sentinel.read_bytes()).hexdigest()==before
path.unlink()
print('PASS: 90s CPU/512MiB memory load, eight 64MiB fsync/read/hash cycles; sentinel '+before,flush=True)
'''
key='/home/fresh10i/.var/app/com.tryomarchy.TryOmarchy/data/claude_key'
subprocess.run(['ssh','-i',key,'-p','2250','-o','BatchMode=yes','-o','UserKnownHostsFile=/tmp/astra-guest-knownhosts','omarchy@127.0.0.1','python3 -'],input=code,text=True,check=True,timeout=130)
