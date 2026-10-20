import json, os, pathlib, re, subprocess, time
root=pathlib.Path('/workspace/dense-measure')
env=dict(os.environ,GOMAXPROCS='4',NW_SERIES_EVIDENCE_DB=str(root/'synthetic.db'),NW_SERIES_CPU_EVIDENCE_DIR=str(root))
bins={'baseline':'/workspace/nw-dense-distribution-base.test','shared':'/workspace/nw-dense-distribution-shared.test'}
args=['-test.run','^TestHistoricalSeriesCPUProfileEvidence$','-test.count=1','-test.v','-test.timeout=5m']
# Construct the fixture once; this first sample is excluded from distributions.
p=subprocess.run([bins['baseline'],*args],env=env,capture_output=True,text=True)
(root/'fixture.log').write_text(p.stdout+p.stderr)
if p.returncode: raise SystemExit(p.returncode)
results=[]
for i in range(30):
    for kind in (['baseline','shared'] if i%2==0 else ['shared','baseline']):
        p=subprocess.run([bins[kind],*args],env=env,capture_output=True,text=True)
        raw=p.stdout+p.stderr
        (root/f'{i+1:02d}-{kind}.log').write_text(raw)
        if p.returncode: raise SystemExit(f'{kind} sample {i+1}: {p.returncode}')
        m=re.search(r'elapsed_ns=(\d+) alloc_bytes=(\d+) allocs=(\d+) facts=(\d+) bytes=(\d+) leaves=(\d+) queries=(\d+) points=(\d+)',raw)
        if not m: raise SystemExit('missing metrics')
        results.append(dict(sample=i+1,path=kind,**dict(zip(['elapsed_ns','alloc_bytes','allocs','facts','bytes','leaves','queries','points'],map(int,m.groups())))))
        (root/'samples.json').write_text(json.dumps(results,indent=2)+'\n')
        print(i+1,kind,results[-1]['elapsed_ns'],flush=True)
