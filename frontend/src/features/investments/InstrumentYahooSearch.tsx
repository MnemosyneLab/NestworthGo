import { useEffect, useId, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { selectFieldClass } from "@/components/ui/select";
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
  const composing = useRef(false);
  const generatedId = useId();
  const inputId = `instrument-yahoo-search-${generatedId}`;
  const listboxId = `${inputId}-options`;
  const [query, setQuery] = useState("");
  const [open, setOpen] = useState(false);
  const [activeIndex, setActiveIndex] = useState(0);
  const debouncedQuery = useDebouncedValue(query, 300);
  const search = useYahooInstrumentSearch(debouncedQuery, instrumentType);
  const hits = search.data ?? EMPTY_HITS;
  const showList = open && query.trim().length >= 2;
  const highlightedIndex = Math.min(activeIndex, Math.max(0, hits.length - 1));
  const activeId = showList && !search.isError && hits[highlightedIndex]
    ? `${listboxId}-${highlightedIndex}`
    : undefined;

  useEffect(() => {
    if (activeId) document.getElementById(activeId)?.scrollIntoView?.({ block: "nearest" });
  }, [activeId]);

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

  return (
    <div className="flex flex-col gap-1.5">
      <Label htmlFor={inputId}>{t(`portfolio.${searchPrefix}Search`)}</Label>
      <div ref={rootRef} className="relative">
        <Input
          id={inputId}
          role="combobox"
          aria-controls={showList ? listboxId : undefined}
          aria-expanded={showList}
          aria-activedescendant={activeId}
          aria-autocomplete="list"
          className={selectFieldClass}
          value={query}
          placeholder={t(`portfolio.${searchPrefix}SearchPlaceholder`)}
          autoComplete="off"
          onFocus={() => setOpen(true)}
          onClick={() => setOpen(true)}
          onBlur={() => setOpen(false)}
          onCompositionStart={() => { composing.current = true; }}
          onCompositionEnd={() => { composing.current = false; }}
          onChange={(event) => {
            setQuery(event.target.value);
            setOpen(true);
            setActiveIndex(0);
          }}
          onKeyDown={(event) => {
            if (!showList) return;
            if ((composing.current || event.nativeEvent.isComposing || event.keyCode === 229) &&
                ["Enter", "Escape", "ArrowDown", "ArrowUp"].includes(event.key)) {
              event.stopPropagation();
              if (event.key === "Enter") event.preventDefault();
              return;
            }
            if (event.key === "ArrowDown") {
              event.preventDefault();
              setActiveIndex((index) => Math.min(index + 1, Math.max(0, hits.length - 1)));
            } else if (event.key === "ArrowUp") {
              event.preventDefault();
              setActiveIndex((index) => Math.max(0, index - 1));
            } else if (event.key === "Enter") {
              event.preventDefault();
              if (!search.isError) choose(hits[highlightedIndex]);
            } else if (event.key === "Escape") {
              event.preventDefault();
              event.stopPropagation();
              setOpen(false);
            }
          }}
        />
        {showList && (
          <div
            id={listboxId}
            role="listbox"
            className="select-panel absolute z-50 mt-1 w-full"
          >
            {search.isFetching && hits.length === 0 ? (
              <div role="status" className="px-2 py-1.5 text-muted-foreground">
                {t(`portfolio.${searchPrefix}SearchSearching`)}
              </div>
            ) : search.isError ? (
              <div role="status" className="px-2 py-1.5 text-destructive">
                {t(`portfolio.${searchPrefix}SearchError`)}
              </div>
            ) : hits.length === 0 ? (
              <div role="status" className="px-2 py-1.5 text-muted-foreground">
                {t(`portfolio.${searchPrefix}SearchNoResults`)}
              </div>
            ) : hits.map((hit, index) => {
              const ticker = hitTicker(hit);
              const subtitle = [instrumentSecondaryName({ name: hit.name, symbol: ticker }), (hit.providerKey === "coingecko" ? hit.providerSymbol : hit.exchange || hit.marketCode)].filter(Boolean).join(" · ");
              return (
                <div
                  key={`${hit.providerSymbol}-${hit.marketCode}-${index}`}
                  id={`${listboxId}-${index}`}
                  role="option"
                  aria-label={hitLabel(hit)}
                  aria-selected={false}
                  data-highlighted={index === highlightedIndex || undefined}
                  className="select-option cursor-default"
                  onMouseDown={(event) => event.preventDefault()}
                  onClick={() => choose(hit)}
                  onMouseEnter={() => setActiveIndex(index)}
                >
                  <span className="min-w-0">
                    <span className="block font-medium">{ticker}</span>
                    {subtitle ? <span className="block text-xs text-muted-foreground">{subtitle}</span> : null}
                  </span>
                </div>
              );
            })}
          </div>
        )}
      </div>
      <p className="text-xs text-muted-foreground">{t(`portfolio.${searchPrefix}SearchHint`)}</p>
    </div>
  );
}
