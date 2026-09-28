import { it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { AccountProductsSection } from './AccountProductsSection';
import i18n from '@/i18n';
const state = vi.hoisted(() => ({ isLoading:false, isError:true, data:undefined as unknown, refetch:vi.fn() }));
vi.mock('@/queries/liquidity', () => ({ useProducts: () => state }));
vi.mock('@/features/liquidity/ProductSheets', () => ({ ProductFormSheet: () => null, ProductDetailSheet: () => null, moneyText: () => '' }));
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
