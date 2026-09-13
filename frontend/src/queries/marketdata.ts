import { useCallback, useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Events } from "@wailsio/runtime";
import { Service as MarketDataService } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/marketdata";
import type { RefreshCompletedPayload, RefreshResultDTO } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/marketdata/models";
import { callService, parseWailsError, translateWailsError } from "@/lib/wails";
import { invalidateMarketDataSync, invalidateQuoteReads, invalidateRefreshAll, invalidateRequiredFX } from "@/queries/invalidation";
import { queryKeys } from "@/queries/keys";
import type {
  SyncJobDTO,
  SyncPlanPreviewDTO,
  SyncRequestDTO,
  SyncStartResultDTO,
} from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/marketdata/models";

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

const SYNC_EVENTS = [
	"marketdata.sync.started",
	"marketdata.sync.progress",
	"marketdata.sync.item",
	"marketdata.sync.completed",
] as const;

// Sync completion is a workspace concern, not a concern of the page that
// happened to start the job. Keep one event subscription alive while the
// workspace is mounted so navigating away from Market Data cannot orphan
// cache invalidation.
type SyncEventClientEntry = {
	references: number;
	cleanups: Array<() => void>;
	cleaned: boolean;
};

const syncEventClients = new Map<ReturnType<typeof useQueryClient>, SyncEventClientEntry>();

function subscribeToSyncEvents(queryClient: ReturnType<typeof useQueryClient>): () => void {
	let entry = syncEventClients.get(queryClient);
	if (entry) {
		entry.references += 1;
	} else {
		entry = {
			references: 1,
			cleaned: false,
			cleanups: SYNC_EVENTS.map((name) =>
			Events.On(name, () => {
				void queryClient.invalidateQueries({ queryKey: queryKeys.marketdata.currentSync });
			}),
			),
		};
		syncEventClients.set(queryClient, entry);
	}
	const registeredEntry = entry;
	let subscribed = true;
	return () => {
		if (!subscribed) return;
		subscribed = false;
		const current = syncEventClients.get(queryClient);
		if (current !== registeredEntry) {
			return;
		}
		current.references -= 1;
		if (current.references <= 0) {
			syncEventClients.delete(queryClient);
			if (!current.cleaned) {
				current.cleaned = true;
				current.cleanups.forEach((cleanup) => cleanup());
			}
		}
	};
}

export function emptySyncJob(job: SyncJobDTO | null | undefined): boolean {
	return !job?.jobId;
}

export function syncJobIsRunning(job: SyncJobDTO | null | undefined): boolean {
	return Boolean(job?.jobId) && job?.outcome === "running";
}

export function useCurrentSyncJob() {
	const queryClient = useQueryClient();
	const query = useQuery({
		queryKey: queryKeys.marketdata.currentSync,
		queryFn: async () => {
			const job = await callService(() => MarketDataService.GetCurrentSyncJob());
			return emptySyncJob(job) ? null : job;
		},
		refetchInterval: (current) => (syncJobIsRunning(current.state.data) ? 2000 : false),
	});

	useEffect(() => {
		return subscribeToSyncEvents(queryClient);
	}, [queryClient]);

	const seenRunning = useRef(false);
	const observedTerminal = useRef("");
	useEffect(() => {
		const job = query.data;
		if (syncJobIsRunning(job)) {
			seenRunning.current = true;
			return;
		}
		if (seenRunning.current) {
			seenRunning.current = false;
			invalidateMarketDataSync(queryClient);
		}
		// A workspace observer can first see a terminal job after the page that
		// started it has been unmounted. Compensate on the first terminal read,
		// and use the stable sequence so polling does not invalidate forever.
		if (job?.jobId && job.outcome !== "running") {
			const terminalSignature = `${job.jobId}:${job.sequence}:${job.outcome}`;
			if (observedTerminal.current !== terminalSignature) {
				observedTerminal.current = terminalSignature;
				invalidateMarketDataSync(queryClient);
			}
		}
	}, [query.data, queryClient]);

	return query;
}

/** Mount once under AppShell so sync observation survives page navigation. */
export function MarketDataSyncWorkspaceObserver() {
	useCurrentSyncJob();
	return null;
}

export function usePreviewMarketDataSync() {
	return useMutation<SyncPlanPreviewDTO, Error, SyncRequestDTO>({
		mutationFn: (request) => callService(() => MarketDataService.PreviewMarketDataSync(request)),
	});
}

export function useStartMarketDataSync() {
	const queryClient = useQueryClient();
	return useMutation<SyncStartResultDTO, Error, SyncRequestDTO>({
		mutationFn: (request) => callService(() => MarketDataService.StartMarketDataSync(request)),
		onSuccess: (result) => {
			if (result.job?.jobId) {
				queryClient.setQueryData(queryKeys.marketdata.currentSync, result.job);
			}
			void queryClient.invalidateQueries({ queryKey: queryKeys.marketdata.currentSync });
		},
	});
}

export function useCancelSyncJob() {
	const queryClient = useQueryClient();
	return useMutation<SyncJobDTO, Error, string>({
		mutationFn: (jobId) => callService(() => MarketDataService.CancelSyncJob(jobId)),
		onSuccess: (job) => {
			queryClient.setQueryData(queryKeys.marketdata.currentSync, emptySyncJob(job) ? null : job);
			void queryClient.invalidateQueries({ queryKey: queryKeys.marketdata.currentSync });
		},
	});
}

export function useMarketDataHealth() {
	return useQuery({
		queryKey: queryKeys.marketdata.health,
		queryFn: () => callService(() => MarketDataService.ScanMarketDataHealth()),
	});
}
