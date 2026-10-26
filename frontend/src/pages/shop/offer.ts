// What the buyer is about to pay for: a stored plan, or the calculator's size.
export interface Offer {
  planId: number;
  name: string;
  price: number;
  currency: string;
  custom?: { gb: number; days: number };
}

// An account chosen up front from "your services" (Mini App sessions).
export interface Preset {
  kind: 'renew' | 'addon';
  email: string;
}
