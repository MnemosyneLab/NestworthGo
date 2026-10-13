import { render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import i18n from "@/i18n";
import { ValuationQualitySummary } from "./ValuationQualitySummary";
import { fullDisplayAmount, valuationTableAmount, type DisplayValuation } from "./valuationQuality";
import { trendQualitySegments } from "@/components/charts/TrendChart";

const full: DisplayValuation = { total:{amount:"200",currency:"CNY"},knownSubtotal:{amount:"200",currency:"CNY"},quality:{availability:"full",coverage:"provisional",includedCount:1,valuedCount:1,unpricedCount:0,provisionalCount:1,carriedCount:1,staleCount:1,oldestMarketDate:"2026-10-09",newestMarketDate:"2026-10-09",reasons:["history_unverified","carried_price","carried_nav"]} };
afterEach(async () => { await i18n.changeLanguage("en"); });
it.each(["en","zh-CN","zh-TW"])("discloses estimates and actual NAV dates without a publication deadline (%s)",async language=>{
 await i18n.changeLanguage(language);render(<ValuationQualitySummary value={full} />);
 const status=screen.getByRole("status");expect(status).toHaveTextContent(i18n.t("valuation.estimated"));expect(status).toHaveTextContent(i18n.t("valuation.latestNAV"));expect(status).toHaveTextContent("2026-10-09");expect(status).not.toHaveTextContent("2026-10-12");
});
it("keeps partial known net contribution separate from a full total and distinguishes null from zero",()=>{
 const partial={...full,total:null,quality:{...full.quality,availability:"partial",unpricedCount:1}};
 expect(fullDisplayAmount(partial,"999")).toBeNull();expect(valuationTableAmount(i18n.t,partial,null,true)).toContain("Known net contribution");
 const none={...partial,knownSubtotal:null,quality:{...partial.quality,availability:"none",valuedCount:0}};
 expect(valuationTableAmount(i18n.t,none,{amount:"0",currency:"CNY"})).toBe("No current value");
 expect(fullDisplayAmount({...full,total:{amount:"0",currency:"CNY"}})).toBe("0");
});
it("dashes estimated edges, preserves verified edges and never crosses a true gap",()=>{
 const segments=trendQualitySegments({key:"net",name:"Net",color:"blue",values:["1","2","3","4",null,"6","7"],pointMeta:[{}, {},{provisional:true},{},undefined,{},{}]});
 expect(segments).toEqual([{indices:[0,1],provisional:false},{indices:[1,2,3],provisional:true},{indices:[5,6],provisional:false}]);
});
