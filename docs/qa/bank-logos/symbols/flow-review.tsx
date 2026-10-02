import {useState} from 'react'; import {createRoot} from 'react-dom/client';
import {QueryClient,QueryClientProvider} from '@tanstack/react-query';
import './index.css'; import i18n from './i18n';
import {DirectoryPage} from './features/directory/DirectoryPage';
import {AccountForm} from './features/accounts/AccountForm';
import {Sheet,SheetContent,SheetTitle} from './components/ui/sheet';
import {queryKeys} from './queries/keys'; import {TEST_CATALOG} from './test/catalog';
const client=new QueryClient({defaultOptions:{queries:{staleTime:Infinity,retry:false,enabled:false}}});
client.setQueryData(queryKeys.catalog.all,TEST_CATALOG);
for(const archived of [true,false]){client.setQueryData(queryKeys.directory.institutions(archived),[{id:'cmb',name:'招商银行',institutionType:'bank',iconKey:'bank-logo:cmbchina'}]);client.setQueryData(queryKeys.directory.members(archived),[{id:'owner',name:'Owner',iconKey:'user'}]);client.setQueryData(queryKeys.directory.groups(archived),[])}
client.setQueryData(queryKeys.settings.supportedCurrencies,['USD','CNY']);
i18n.changeLanguage('zh-CN');
const record={account:{id:'account',name:'招商账户',accountType:'bank_account',balanceSheetRole:'asset',trackingMode:'balance',defaultCurrency:'CNY',iconKey:'bank-logo:cmbchina',institutionId:'cmb',includeInNetWorth:true,includeInPortfolio:true,includeInLiquidAssets:true},owners:[{memberId:'owner'}],ownership:[]};
function App(){const [open,setOpen]=useState(false);return <QueryClientProvider client={client}><main className="bg-background text-foreground min-h-screen p-6"><DirectoryPage/><button className="mt-8" onClick={()=>setOpen(true)}>Review account edit</button><Sheet open={open} onOpenChange={setOpen}><SheetContent><SheetTitle>账户设置</SheetTitle><AccountForm record={record as never} onSubmit={()=>{}} submitLabel="保存" isSubmitting={false}/><button onClick={()=>setOpen(false)}>取消</button></SheetContent></Sheet></main></QueryClientProvider>};createRoot(document.getElementById('root')!).render(<App/>);
