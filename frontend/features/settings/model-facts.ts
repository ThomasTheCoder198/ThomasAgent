import type { Model } from "@/features/registry/types";

const THOUSAND = 1000;

/**
 * "200k ctx · $3 / $15 per 1M" from what the provider API gave (O8); unknown facts are simply left out.
 * `perMillion` is the localized "per 1M" that names the price unit.
 */
export function modelFacts(m: Model, perMillion?: string): string {
  const parts: string[] = [];
  if (m.contextWindow) parts.push(`${Math.round(m.contextWindow / THOUSAND)}k ctx`);
  if (m.inputPricePerMTok !== undefined && m.outputPricePerMTok !== undefined) {
    const price = `$${m.inputPricePerMTok} / $${m.outputPricePerMTok}`;
    parts.push(perMillion ? `${price} ${perMillion}` : price);
  }
  return parts.join(" · ");
}
