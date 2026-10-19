import csv,math,pathlib,re,statistics
root=pathlib.Path(__file__).resolve().parent.parent
def ms(v):
 m=re.fullmatch(r'([0-9.]+)(ns|µs|ms|s)',v);return float(m[1])*{'ns':1e-6,'µs':1e-3,'ms':1,'s':1000}[m[2]]
def q(v,p):return sorted(v)[math.ceil(len(v)*p)-1]
s=(root/'after-real-epochs64.log').read_text().split('=== RUN   TestReviewPairedPerformanceEvidence/no-op\n')[1].split('=== RUN   TestReviewPairedPerformanceEvidence/clock-only\n')[0]
rows=[]
for line in s.splitlines():
 if 'full reconcile path=' not in line:continue
 m=re.search(r'path=(\d+) sample=(\d+) elapsed=(\S+) pairDays=(\d+) allocatedBytes=(\d+) allocations=(\d+).*?queries=(\d+) returnedRows=(\d+) Query-to-Rows.Close=(\S+) execs=(\d+) writeStatements=(\d+) rowsAffected=(\d+) driverExecTime=(\S+)',line)
 assert m,line
 path,sample,total,days,b,alloc,queries,returned,query,execs,writes,affected,exectime=m.groups()
 rows.append(dict(path=int(path),sample=int(sample),order='first' if int(path)==int(sample)%2 else 'second',total_ms=ms(total),allocated_bytes=int(b),allocations=int(alloc),query_ms=ms(query),exec_ms=ms(exectime),queries=int(queries),returned=int(returned),execs=int(execs),writes=int(writes),affected=int(affected)))
out=pathlib.Path(__file__).resolve().parent
with (out/'original-noop-samples.csv').open('w') as f:
 w=csv.DictWriter(f,fieldnames=list(rows[0]),lineterminator='\n');w.writeheader();w.writerows(rows)
for path in [0,1]:
 a=[r for r in rows if r['path']==path]
 print('PATH',path,'n',len(a),'median/p95',q([r['total_ms'] for r in a],.5),q([r['total_ms'] for r in a],.95))
 for key in ['allocated_bytes','allocations','query_ms','exec_ms']:
  v=[r[key] for r in a]; print(key,'median',q(v,.5),'range',min(v),max(v))
 for order in ['first','second']:
  v=[r['total_ms'] for r in a if r['order']==order];print(order,'n',len(v),'mean',statistics.mean(v),'median',q(v,.5))
 print('top3', sorted(a,key=lambda r:r['total_ms'],reverse=True)[:3])
