import React, { useState } from 'react';
import { createRoot } from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { NavigationContext } from '@/app/NavigationContext';
import { InstrumentDetailPage } from '@/features/investments/InstrumentDetailPage';
import { MarketDataPage } from '@/features/marketdata/MarketDataPage';
import { HoldingsTab } from '@/features/investments/HoldingsTab';
import type { HealthFocus, InstrumentDetailContext, NavigationTarget } from '@/app/navigation';
import { queryKeys } from '@/queries/keys';
import '@/i18n';
import '@fontsource-variable/plus-jakarta-sans';
import '@/index.css';

const client = new QueryClient({ defaultOptions: { queries: { staleTime: Infinity, retry: false } } });
const account = { id: 'synthetic-broker', name: 'Demo Brokerage', accountType: 'brokerage', trackingMode: 'holdings', defaultCurrency: 'USD' };
const old = { id: 'synthetic-old', name: 'Archive Brokerage', accountType: 'brokerage', trackingMode: 'holdings', defaultCurrency: 'USD', archivedAt: '2026-01-01' };
const instrument = { id: 'synthetic-fund', householdId: 'synthetic-only', name: 'Sample Index Fund', symbol: 'SAMPLE', type: 'fund', quoteCurrency: 'USD', quoteSource: 'manual', createdAt: '2026-01-01', updatedAt: '2026-01-01' };
const cash = (amount: string) => ({ amount, currency: 'USD' });
const makeHolding = (accountId: string, accountName: string, quantity: string, cost: string, value: string) => ({ holdingId: `synthetic-${accountId}`, accountId, accountName, amounts: { quantity, totalCost: cash(cost), currentValue: cash(value), unrealizedGain: cash(String(Number(value)-Number(cost))) } });
const current = makeHolding(account.id, account.name, '12', '1200', '1440');
const archived = makeHolding(old.id, old.name, '3', '300', '330');
const group = (members: typeof current[]) => [{ key: 'fund', instruments: [{ instrumentId: instrument.id, name: instrument.name, symbol: instrument.symbol, type: 'fund', quoteCurrency: 'USD', quantityUnit: 'units', archived: false, holdings: members, amounts: { quantity: String(members.reduce((sum,m) => sum+Number(m.amounts.quantity),0)), totalCost: cash(String(members.reduce((sum,m) => sum+Number(m.amounts.totalCost.amount),0))), currentValue: cash(String(members.reduce((sum,m) => sum+Number(m.amounts.currentValue.amount),0))), unrealizedGain: cash(String(members.reduce((sum,m) => sum+Number(m.amounts.unrealizedGain.amount),0))) } }], subtotals: [] }];
client.setQueryData(queryKeys.instruments.list(false), [instrument]);
client.setQueryData(queryKeys.instruments.list(true), [instrument]);
client.setQueryData(queryKeys.accounts.list({}), [{ account, ownership: [], latestValue: null }]);
client.setQueryData(queryKeys.accounts.list({ includeArchived: true }), [{ account, ownership: [], latestValue: null }, { account: old, ownership: [], latestValue: null }]);
client.setQueryData(queryKeys.accounts.valuations({}), []);
client.setQueryData(queryKeys.holdings.byAccounts([account.id]), { [account.id]: [] });
client.setQueryData([...queryKeys.analytics.instrumentHoldings, 'groups', ''], group([current]));
client.setQueryData([...queryKeys.analytics.instrumentHoldings, 'groups', account.id], group([current]));
client.setQueryData([...queryKeys.analytics.instrumentHoldings, 'groups', old.id], group([archived]));
client.setQueryData(queryKeys.history.origin, { id: 'synthetic-history', timezone: 'UTC', startedAt: '2026-01-01' });
client.setQueryData(queryKeys.liquidity.products('', true), []);
client.setQueryData(queryKeys.quote.instrument.current(instrument.id), { unitPrice: '120', currency: 'USD', sourceKind: 'manual', asOf: '2026-10-15T12:00:00Z' });
for (const range of ['all', '2026-09-01:2026-09-30']) client.setQueryData(queryKeys.quote.instrument.series(instrument.id, range, 'all'), { range, points: [], observations: [], outsideRange: false });
client.setQueryData(queryKeys.quote.fx.preferences, [{ currencyA: 'CNY', currencyB: 'USD', sourceKind: 'manual', sourceKey: 'manual' }]);
client.setQueryData(queryKeys.quote.fx.current('CNY', 'USD'), { baseCurrency: 'CNY', quoteCurrency: 'USD', rate: '0.14', sourceKind: 'manual', asOf: '2026-10-15T12:00:00Z' });
client.setQueryData(queryKeys.settings.all, { timezone: 'UTC', language: 'en', appearance: 'light', accent: 'blue' });
client.setQueryData(queryKeys.household.bootstrap, { household: { id: 'synthetic-only', name: 'Demo Household', baseCurrency: 'USD' }, members: [], institutions: [], groups: [] });
client.setQueryData(queryKeys.history.mutationAllowed, {});
client.setQueryData(queryKeys.settings.supportedCurrencies, ['USD']);
client.setQueryData(queryKeys.catalog.all, { currencies: ['USD', 'CNY'], quoteSources: ['manual', 'provider', 'agent'], instrumentTypes: ['fund'] });
client.setQueryData(queryKeys.overview.all, { missingInputs: [] });
client.setQueryData(queryKeys.marketdata.health, { issueCount: 0, issues: [] });
client.setQueryData(queryKeys.marketdata.currentSync, { jobId: '' });
const view = new URLSearchParams(location.search).get('view') ?? 'holdings';
function Preview() {
  const params = new URLSearchParams(location.search);
  const [context, setContext] = useState<InstrumentDetailContext>({ instrumentId: instrument.id, accountId: view === 'archived' ? old.id : undefined,
    section: 'holdings', from: params.get('from') ?? undefined, to: params.get('to') ?? undefined });
  const [page, setPage] = useState(view === 'market' ? 'market' : view === 'detail' || view === 'archived' ? 'detail' : 'holdings');
  const [marketFocus, setMarketFocus] = useState<HealthFocus>();
  const [trail, setTrail] = useState<string[]>([]);
  const open = (target: NavigationTarget) => {
    setTrail((items) => [...items, page]);
    if (target.page === 'market-data') { setMarketFocus(target.focus); setPage('market'); }
    if (target.page === 'instrument-detail') { setContext({ ...target, section: target.section ?? 'holdings' }); setPage('detail'); }
  };
  const back = () => { const next = trail.at(-1); if (next) { setTrail(trail.slice(0, -1)); setPage(next); } };
  return <NavigationContext.Provider value={{ open, openHealth: () => {} }}><main className="mx-auto max-w-6xl p-8">
    <div className="mb-5 rounded-lg border border-amber-300 bg-amber-50 px-4 py-2 text-sm text-amber-900">Web preview · Synthetic fixtures only · No ledger connected</div>
    {trail.length > 0 && <button className="mb-4 rounded border px-3 py-2" onClick={back}>Back to previous page</button>}
    <div hidden={page !== 'holdings'}><HoldingsTab /></div>
    <div hidden={page !== 'detail'}><InstrumentDetailPage context={context} onContextChange={setContext} /></div>
    <div hidden={page !== 'market'}><MarketDataPage focus={marketFocus} /></div>
  </main></NavigationContext.Provider>;
}
createRoot(document.getElementById('root')!).render(<QueryClientProvider client={client}><Preview /></QueryClientProvider>);
