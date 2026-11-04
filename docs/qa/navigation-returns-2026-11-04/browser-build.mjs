// Opt-in synthetic HTTP harness, adapted from the maintained same-page fixture.
// Seed an explicit historical origin; embed the unchanged production frontend.
import fs from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { spawnSync } from 'node:child_process';
const repo = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../../..');
const frontend = path.join(repo, 'frontend');
const root = path.resolve(process.argv[2] || '');
if (!process.argv[2] || !(root.startsWith('/tmp/') || root.startsWith('/workspace/scratch/'))) throw new Error('Use a fresh synthetic directory under /tmp or /workspace/scratch');
await fs.mkdir(root, { recursive: true });
await fs.writeFile(path.join(root, 'SYNTHETIC_TEST_ONLY'), 'Same-page navigation layout fixture\n');
const temporary = await fs.mkdtemp(path.join(repo, '.same-page-seed-'));
const go = `package main
import("context";"fmt";"os";"path/filepath";"time";"github.com/waltwang/nestworth-go/internal/application";"github.com/waltwang/nestworth-go/internal/domain";"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite")
func must[T any](v T,e error)T{if e!=nil{panic(e)};return v}
func main(){root:=os.Args[1];dbPath:=filepath.Join(root,"household.db");if _,e:=os.Stat(dbPath);!os.IsNotExist(e){panic("fresh database required")};db:=must(sqlite.Open(dbPath));defer db.Close();svc:=application.NewService(sqlite.NewRepository(db));ctx:=context.Background();svc.SetClock(func()time.Time{return time.Date(2026,11,1,0,0,0,0,time.UTC)});if e:=svc.CompleteOnboarding(ctx,application.OnboardingInput{HouseholdName:"Same-page navigation synthetic QA",BaseCurrency:"USD",MemberNames:[]string{"Synthetic member"}});e!=nil{panic(e)};b:=must(svc.Bootstrap(ctx));owners:=[]domain.MemberID{b.Members[0].ID};for i:=0;i<60;i++{name:=fmt.Sprintf("QA %02d Cash",i);if i==20{name="QA 20 Short"};must(svc.CreateAccount(ctx,application.AccountInput{Name:name,AccountType:"bank_account",BalanceSheetRole:"asset",TrackingMode:"balance",DefaultCurrency:"USD",InitialAmount:"1",IncludeInNetWorth:true,OwnerIDs:owners}))};a:=must(svc.CreateAccount(ctx,application.AccountInput{Name:"QA Long holdings",AccountType:"brokerage",BalanceSheetRole:"asset",TrackingMode:"holdings",DefaultCurrency:"USD",IncludeInNetWorth:true,IncludeInPortfolio:true,OwnerIDs:owners}));for i:=0;i<40;i++{instrument:=must(svc.CreateInstrument(ctx,application.InstrumentInput{Name:fmt.Sprintf("Synthetic holding %02d",i),Type:"stock",QuoteCurrency:"USD",QuoteSource:"manual"}));must(svc.AppendManualInstrumentQuote(ctx,instrument.ID,"1","2026-10-31T00:00:00Z",false));must(svc.CreateHolding(ctx,application.HoldingInput{AccountID:a.Account.ID.String(),InstrumentID:instrument.ID.String(),Quantity:"1"}))};must(svc.StartHistory(ctx,"UTC"));fmt.Println("Synthetic fixture: 61 accounts / 40 holdings")}
`;
let replaced = false;
try {
  await fs.writeFile(path.join(temporary, 'main.go'), go);
  const seed = spawnSync('go', ['run', path.join(temporary, 'main.go'), root], { cwd: repo, env: { ...process.env, CGO_ENABLED: '0' }, stdio: 'inherit' });
  if (seed.status) throw new Error('Synthetic fixture creation failed');
  const { build } = await import(pathToFileURL(path.join(frontend, 'node_modules/vite/dist/node/index.js')));
  await build({ root: frontend, configFile: path.join(frontend, 'vite.config.ts'), build: { outDir: path.join(root, 'bundle'), emptyOutDir: true } });
  await fs.rename(path.join(frontend, 'dist'), path.join(root, 'production-dist'));
  replaced = true;
  await fs.cp(path.join(root, 'bundle'), path.join(frontend, 'dist'), { recursive: true });
  const compiled = spawnSync('go', ['build', '-tags', 'server,production', '-o', path.join(root, 'server'), './cmd/nestworth'], { cwd: repo, env: { ...process.env, CGO_ENABLED: '0' }, stdio: 'inherit' });
  if (compiled.status) throw new Error('Production HTTP build failed');
} finally {
  if (replaced) {
    await fs.rm(path.join(frontend, 'dist'), { recursive: true, force: true });
    await fs.rename(path.join(root, 'production-dist'), path.join(frontend, 'dist'));
  }
  await fs.rm(temporary, { recursive: true, force: true });
}
