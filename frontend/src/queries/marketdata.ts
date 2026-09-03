import { useCallback, useEffect, useRef, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Events } from "@wailsio/runtime";
import { Service as MarketDataService } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/marketdata";
import type { RefreshCompletedPayload, RefreshResultDTO } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/marketdata/models";
import { callService, parseWailsError, translateWailsError } from "@/lib/wails";
import { invalidateQuoteReads, invalidateRefreshAll, invalidateRequiredFX } from "@/queries/invalidation";

const REFRESH_COMPLETED_EVENT = "marketdata.refresh.completed" as const;

export type RefreshOperationStatus = "idle" | "refreshing" | "completed" | "failed" | "cancelled";

type AsyncRefreshOptions<TInput> = {
	start: (requestId: string, input: TInput) => Promise<void>;
	invalidate: (queryClient: ReturnType<typeof useQueryClient>, input: TInput, result: RefreshResultDTO) => void;
};

type PendingRefresh = {
	requestId: string;
	cleanup: () => void;
	resolve: (result: RefreshResultDTO | undefined) => void;
	reject: (error: Error) => void;
	cancelled: boolean;
};

function refreshEventError(raw: string): Error {
	const wireError = parseWailsError(raw);
	const error = new Error(translateWailsError(wireError));
	(error as Error & { wireError: typeof wireError }).wireError = wireError;
	return error;
}

function resultFailed(result: RefreshResultDTO): boolean {
	return Boolean(result.items?.some((item) => item.status === "failed" || item.status === "rate_limited" || item.status === "skipped"));
}

function useAsyncRefresh<TInput>({ start, invalidate }: AsyncRefreshOptions<TInput>) {
	const queryClient = useQueryClient();
	const pending = useRef<PendingRefresh | null>(null);
	const [operationStatus, setOperationStatus] = useState<RefreshOperationStatus>("idle");

	const cancel = useCallback(() => {
		const current = pending.current;
		if (!current) return;
		current.cancelled = true;
		current.cleanup();
		pending.current = null;
		setOperationStatus("cancelled");
		void callService(() => MarketDataService.CancelRefresh(current.requestId)).catch(() => {
			// The Go cancellation command is idempotent; the listener is already detached.
		});
		current.resolve(undefined);
	}, []);

	const mutation = useMutation<RefreshResultDTO | undefined, Error, TInput>({
		mutationFn: async (input) => {
			const requestId = crypto.randomUUID();
			const completion = new Promise<RefreshResultDTO | undefined>((resolve, reject) => {
				const cleanup = Events.On(REFRESH_COMPLETED_EVENT, (event) => {
					const payload: RefreshCompletedPayload = event.data;
					if (payload.requestId !== requestId || pending.current?.cancelled) return;
					pending.current = null;
					cleanup();
					if (payload.status === "cancelled") {
						setOperationStatus("cancelled");
						resolve(undefined);
						return;
					}
					if (payload.error) {
						setOperationStatus("failed");
						reject(refreshEventError(payload.error));
						return;
					}
					if (!payload.result) {
						setOperationStatus("failed");
						reject(new Error("Refresh completed without a result"));
						return;
					}
					setOperationStatus(resultFailed(payload.result) ? "failed" : "completed");
					resolve(payload.result);
				});
				pending.current = { requestId, cleanup, resolve, reject, cancelled: false };
			});
			setOperationStatus("refreshing");
			try {
				await callService(() => start(requestId, input));
				return await completion;
			} catch (error) {
				pending.current?.cleanup();
				pending.current = null;
				setOperationStatus("failed");
				throw error;
			}
		},
		onSuccess: (result, input) => {
			if (result) invalidate(queryClient, input, result);
		},
	});

	useEffect(() => () => cancel(), [cancel]);
	return { ...mutation, operationStatus, refreshing: operationStatus === "refreshing", cancel };
}

export function useRefreshAll() {
	return useAsyncRefresh<void>({
		start: (requestId) => MarketDataService.StartRefreshAll(requestId),
		invalidate: (queryClient) => invalidateRefreshAll(queryClient),
	});
}

export function useRefreshRequiredFX() {
	return useAsyncRefresh<void>({
		start: (requestId) => MarketDataService.StartRefreshRequiredFX(requestId),
		invalidate: (queryClient) => invalidateRequiredFX(queryClient),
	});
}

export function useRefreshMissingOrStale() {
	return useAsyncRefresh<void>({
		start: (requestId) => MarketDataService.StartRefreshMissingOrStale(requestId),
		invalidate: (queryClient) => invalidateRefreshAll(queryClient),
	});
}

export function useRefreshInstrument() {
	return useAsyncRefresh<string>({
		start: (requestId, instrumentId) => MarketDataService.StartRefreshInstrument(requestId, instrumentId),
		invalidate: (queryClient, instrumentId) => invalidateQuoteReads(queryClient, instrumentId),
	});
}

export function useRefreshFX() {
	return useAsyncRefresh<{ currencyA: string; currencyB: string }>({
		start: (requestId, pair) => MarketDataService.StartRefreshFX(requestId, pair.currencyA, pair.currencyB),
		invalidate: (queryClient) => invalidateRequiredFX(queryClient),
	});
}
