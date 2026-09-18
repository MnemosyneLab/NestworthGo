import { useEffect, useId, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { cn } from "@/lib/utils";
import { instrumentDisplayLabel, instrumentSecondaryName } from "@/lib/instrumentDisplay";
import { useYahooInstrumentSearch } from "@/queries/marketdata";
import type { InstrumentSearchHitDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/marketdata/models";

const EMPTY_HITS: InstrumentSearchHitDTO[] = [];

function useDebouncedValue(value: string, delayMs: number): string {
  const [debounced, setDebounced] = useState(value);
  useEffect(() => {
    const handle = window.setTimeout(() => setDebounced(value), delayMs);
    return () => window.clearTimeout(handle);
  }, [value, delayMs]);
  return debounced;
}

function hitTicker(hit: InstrumentSearchHitDTO): string {
  return instrumentDisplayLabel({ name: hit.name, symbol: hit.symbol || hit.providerSymbol });
}

function hitLabel(hit: InstrumentSearchHitDTO): string {
  const ticker = hitTicker(hit);
  const venue = hit.providerKey === "coingecko" ? hit.providerSymbol : hit.exchange || hit.marketCode;
  const name = instrumentSecondaryName({ name: hit.name, symbol: ticker });
  return [ticker, name, venue].filter(Boolean).join(" · ");
}

export function InstrumentYahooSearch({
  instrumentType,
  onSelect,
}: {
  instrumentType: string;
  onSelect: (hit: InstrumentSearchHitDTO) => void;
}) {
  const { t } = useTranslation();
  const searchPrefix = instrumentType === "crypto" ? "crypto" : "yahoo";
  const rootRef = useRef<HTMLDivElement>(null);
  const generatedId = useId();
  const inputId = `instrument-yahoo-search-${generatedId}`;
  const listboxId = `${inputId}-options`;
  const [query, setQuery] = useState("");
  const [open, setOpen] = useState(false);
  const [activeIndex, setActiveIndex] = useState(0);
  const debouncedQuery = useDebouncedValue(query, 300);
  const search = useYahooInstrumentSearch(debouncedQuery, instrumentType);
  const hits = search.data ?? EMPTY_HITS;
  const selectedIndex = Math.min(activeIndex, Math.max(0, hits.length - 1));

  useEffect(() => {
    const handlePointerDown = (event: PointerEvent) => {
      if (!rootRef.current?.contains(event.target as Node)) {
        setOpen(false);
      }
    };
    document.addEventListener("pointerdown", handlePointerDown);
    return () => document.removeEventListener("pointerdown", handlePointerDown);
  }, []);

  const choose = (hit: InstrumentSearchHitDTO | undefined) => {
    if (!hit) return;
    onSelect(hit);
    setQuery(hitLabel(hit));
    setOpen(false);
  };

  const showList = open && query.trim().length >= 2;

  return (
    <div className="flex flex-col gap-1.5">
      <Label htmlFor={inputId}>{t(`portfolio.${searchPrefix}Search`)}</Label>
      <div ref={rootRef} className="relative">
        <Input
          id={inputId}
          role="combobox"
          aria-controls={listboxId}
          aria-expanded={showList}
          aria-autocomplete="list"
          value={query}
          placeholder={t(`portfolio.${searchPrefix}SearchPlaceholder`)}
          autoComplete="off"
          onFocus={() => setOpen(true)}
          onChange={(event) => {
            setQuery(event.target.value);
            setOpen(true);
            setActiveIndex(0);
          }}
          onKeyDown={(event) => {
            if (!showList) return;
            if (event.key === "ArrowDown") {
              event.preventDefault();
              setActiveIndex((index) => Math.min(index + 1, Math.max(0, hits.length - 1)));
            } else if (event.key === "ArrowUp") {
              event.preventDefault();
              setActiveIndex((index) => Math.max(0, index - 1));
            } else if (event.key === "Enter") {
              event.preventDefault();
              choose(hits[selectedIndex]);
            } else if (event.key === "Escape") {
              event.preventDefault();
              setOpen(false);
            }
          }}
        />
        {showList && (
          <ul
            id={listboxId}
            role="listbox"
            className="absolute z-50 mt-1 max-h-60 w-full overflow-y-auto rounded-lg border border-border bg-card p-1 text-sm shadow-lg"
          >
            {search.isFetching && hits.length === 0 ? (
              <li role="presentation" className="rounded-md px-3 py-2 text-muted-foreground">
                {t(`portfolio.${searchPrefix}SearchSearching`)}
              </li>
            ) : search.isError ? (
              <li role="presentation" className="rounded-md px-3 py-2 text-destructive">
                {t(`portfolio.${searchPrefix}SearchError`)}
              </li>
            ) : hits.length === 0 ? (
              <li role="option" aria-disabled="true" className="rounded-md px-3 py-2 text-muted-foreground">
                {t(`portfolio.${searchPrefix}SearchNoResults`)}
              </li>
            ) : hits.map((hit, index) => {
              const ticker = hitTicker(hit);
              const subtitle = [instrumentSecondaryName({ name: hit.name, symbol: ticker }), (hit.providerKey === "coingecko" ? hit.providerSymbol : hit.exchange || hit.marketCode)].filter(Boolean).join(" · ");
              return (
              <li
                key={`${hit.providerSymbol}-${hit.marketCode}-${index}`}
                role="option"
                aria-label={hitLabel(hit)}
                aria-selected={index === selectedIndex}
                className={cn("cursor-pointer rounded-md px-3 py-2 text-foreground", index === selectedIndex && "bg-muted")}
                onMouseDown={(event) => event.preventDefault()}
                onClick={() => choose(hit)}
                onMouseEnter={() => setActiveIndex(index)}
              >
                <span className="block font-medium">{ticker}</span>
                {subtitle ? <span className="block truncate text-xs text-muted-foreground">{subtitle}</span> : null}
              </li>
              );
            })}
          </ul>
        )}
      </div>
      <p className="text-xs text-muted-foreground">{t(`portfolio.${searchPrefix}SearchHint`)}</p>
    </div>
  );
}
