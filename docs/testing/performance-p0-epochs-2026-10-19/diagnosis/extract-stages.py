import csv,math,pathlib,re,statistics
root=pathlib.Path(__file__).resolve().parent
def ms(v):
 m=re.fullmatch(r'([0-9.]+)(ns|µs|ms|s)',v);return float(m[1])*{'ns':1e-6,'µs':1e-3,'ms':1,'s':1000}[m[2]]
def q(v,p):return sorted(v)[math.ceil(len(v)*p)-1]
rows={}
text=(root/'stages-epochs64.log').read_text()
for line in text.splitlines():
 if 'stage path=' in line:
  row={k:int(v) for k,v in re.findall(r'(\w+)=(-?\d+)',line)};row['order']='first' if row['path']==row['sample']%2 else 'second';rows[row['path'],row['sample']]=row
 if 'stage SQL path=' in line:
  path,sample=map(int,re.search(r'path=(\d+) sample=(\d+)',line).groups()); row=rows[path,sample]
  for phase in ['capture','publish']:
   block=re.search(phase+r'=\{([^}]+)\}',line)[1]
   for key,pattern in [('queries',r'queries=(\d+)'),('returned',r'returnedRows=(\d+)'),('execs',r'execs=(\d+)'),('writes',r'writeStatements=(\d+)'),('affected',r'rowsAffected=(\d+)')]:row[phase+'_'+key]=int(re.search(pattern,block)[1])
   row[phase+'_query_ms']=ms(re.search(r'Query-to-Rows.Close=(\S+)',block)[1]);row[phase+'_exec_ms']=ms(re.search(r'driverExecTime=(\S+)',block)[1])
assert len(rows)==60 and all(r['window_queries']==4 and r['window_rows']==r['reused']==12 for r in rows.values())
with (root/'stage-samples.csv').open('w') as f:
 w=csv.DictWriter(f,fieldnames=list(next(iter(rows.values()))),lineterminator='\n');w.writeheader();w.writerows(rows.values())
summary=[]
for path in [0,1]:
 a=[r for r in rows.values() if r['path']==path]
 print('PATH',path,'n',len(a))
 for key in ['total_ns','capture_ns','prepare_admission_ns','publish_ns','residual_ns','window_ns','allocated_bytes','allocations','gc_cycles','gc_pause_ns','capture_query_ms','publish_query_ms','publish_exec_ms']:
  scale=1e6 if key.endswith('_ns') else 1
  v=[r[key]/scale for r in a]; med,p95=q(v,.5),q(v,.95)
  summary.append(dict(path=path,metric=(key[:-3]+"_ms" if key.endswith("_ns") else key),n=len(v),median=med,p95=p95,min=min(v),max=max(v),mean=statistics.mean(v)))
  print(key, 'median/p95',round(med,6),round(p95,6),'range',round(min(v),6),round(max(v),6))
 print('top3',sorted(a,key=lambda r:r['total_ns'],reverse=True)[:3])
with (root/'stage-distributions.csv').open('w') as f:
 w=csv.DictWriter(f,fieldnames=list(summary[0]),lineterminator='\n');w.writeheader();w.writerows(summary)
for key in ['total_ns','capture_ns','prepare_admission_ns','publish_ns','window_ns','allocated_bytes','allocations']:
 d=[rows[1,s][key]-rows[0,s][key] for s in range(30)];scale=1e6 if key.endswith('_ns') else 1
 print('paired',key,'mean',statistics.mean(d)/scale,'SD',statistics.stdev(d)/scale,'candidate faster/lower',sum(x<0 for x in d))
