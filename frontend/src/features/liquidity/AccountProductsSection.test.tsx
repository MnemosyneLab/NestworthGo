import { it, expect, vi, afterEach } from 'vitest';
import { render, screen, fireEvent, within } from '@testing-library/react';
import { AccountProductsSection } from './AccountProductsSection';
import i18n from '@/i18n';
const state = vi.hoisted(() => ({ isLoading:false, isError:true, data:undefined as unknown, refetch:vi.fn() }));
vi.mock('@/queries/liquidity', () => ({ useProducts: () => state }));
vi.mock('@/features/liquidity/ProductSheets', async () => ({ ProductFormSheet: () => null, ProductDetailSheet: ({ productId }: { productId: string | null }) => productId ? <div data-testid="opened-product">{productId}</div> : null, moneyText: (await import('./liquidityDisplay')).moneyText }));
afterEach(async () => { await i18n.changeLanguage('en'); });
const record = { account:{ id:'account',name:'Bank',defaultCurrency:'USD',trackingMode:'holdings',balanceSheetRole:'asset',accountType:'bank',archivedAt:null } } as Parameters<typeof AccountProductsSection>[0]['record'];
it('shows a retryable error when products cannot load', () => {
  state.isError=true;state.data=undefined;
  render(<AccountProductsSection record={record} />);
  expect(screen.getByRole('alert')).toBeInTheDocument();
  fireEvent.click(screen.getByRole('button', {name: 'Try again'}));
  expect(state.refetch).toHaveBeenCalledOnce();
  expect(screen.queryByText(i18n.t('availableFunds.unsupportedPartial'))).not.toBeInTheDocument();
});
it('keeps closed product history visible in archived accounts', () => {
  state.isError=false;state.data=[{product:{id:'closed',name:'Closed deposit',kind:'term_deposit',principal:{amount:'100',currency:'USD'},currentValue:null,policy:{},displayState:'settled'}}];
  render(<AccountProductsSection record={{...record,account:{...record.account,archivedAt:'2026-09-22T00:00:00Z'}}} />);
  expect(screen.getByText('Closed deposit')).toBeInTheDocument();
  expect(screen.queryByRole('button', {name: 'Add product'})).not.toBeInTheDocument();
  expect(screen.queryByText(i18n.t('availableFunds.simpleAccountHint'))).not.toBeInTheDocument();
});

it.each(['en', 'zh-CN', 'zh-TW'])('preserves product columns, full values, date fallbacks and row navigation in %s', async (language) => {
  await i18n.changeLanguage(language);
  const amount = '123456789012345.67';
  state.isError = false;
  state.data = [
    { product: { id: 'active', name: 'Deposit', kind: 'term_deposit', principal: { amount: '3000', currency: 'CNY' }, currentValue: { amount: '3000', currency: 'CNY' }, maturityOn: '2026-10-31', policy: {}, displayState: 'active' } },
    { product: { id: 'due', name: '长金额产品', kind: 'locked_product', principal: { amount, currency: 'CNY' }, currentValue: { amount, currency: 'CNY' }, maturityOn: null, policy: { unlockOn: '2027-01-31' }, displayState: 'due_unconfirmed' } },
    { product: { id: 'closed', name: 'Closed deposit', kind: 'term_deposit', principal: { amount: '100', currency: 'CNY' }, currentValue: null, maturityOn: null, policy: {}, displayState: 'settled' } },
  ];
  render(<AccountProductsSection record={record} />);
  const table = screen.getByRole('table', { name: i18n.t('availableFunds.products') });
  expect(within(table).getAllByRole('columnheader').map(cell => cell.textContent)).toEqual(
    ['name', 'asset', 'principal', 'currentContractValue', 'maturityOn'].map(key => i18n.t(`availableFunds.${key}`)),
  );
  const rows = within(table).getAllByRole('row').slice(1);
  expect(rows.map(row => within(row).getByRole('button').textContent)).toEqual(['Deposit', '长金额产品', 'Closed deposit']);
  const { formatAmount } = await import('@/lib/money');
  const cells = rows.map(row => within(row).getAllByRole('cell'));
  expect(cells[0][3]).toHaveTextContent(formatAmount('3000', 'CNY'));
  expect(cells[0][4]).toHaveTextContent('2026-10-31');
  expect(cells[1][2]).toHaveTextContent(formatAmount(amount, 'CNY'));
  expect(cells[1][3]).toHaveTextContent(formatAmount(amount, 'CNY'));
  expect(cells[1][4]).toHaveTextContent('2027-01-31');
  expect(cells[2][3]).toHaveTextContent(i18n.t('availableFunds.unknownAmount'));
  expect(cells[2][4]).toHaveTextContent(i18n.t('availableFunds.unknownAmount'));
  fireEvent.click(within(rows[1]).getByRole('button'));
  expect(screen.getByTestId('opened-product')).toHaveTextContent('due');
});
