// Shapes returned by the Go API (see backend/openapi/openapi.yaml).

import type { FitResult, FitRule } from "./fit";

export type SocialLink = { network: "whatsapp" | "instagram" | "tiktok" | "facebook" | "youtube" | "x"; url: string };
export type DeliveryZone = {
  key: string;
  method: "pickup" | "local_delivery" | "courier";
  label: string;
  feeMinor: number;
  description: string;
};

export type Business = {
  name: string;
  tagline: string;
  legalName?: string;
  registrationNumber?: string;
  taxId?: string;
  logoUrl: string;
  phone: string;
  whatsapp: string;
  email: string;
  address: {
    line1: string;
    line2: string;
    city: string;
    region: string;
    country: string;
    postalCode: string;
    mapUrl: string;
  };
  openingHours: { days: string; hours: string }[] | null;
  currency: string;
  locale: string;
  timezone: string;
  countryCode: string;
  social: SocialLink[] | null;
  delivery: DeliveryZone[];
  quoteValidityDays: number;
  depositPercentBp: number;
  taxLabel: string;
  taxRateBp: number;
  pricesIncludeTax: boolean;
};

export type Flags = Record<
  | "studio"
  | "appointments"
  | "customer_accounts"
  | "support_inbox"
  | "guest_checkout"
  | "online_payments"
  | "reference_analysis"
  | "photo_body_estimation",
  boolean
>;

export type PublicConfig = { business: Business; flags: Flags; demoContent?: boolean };

export type Me = {
  id: string;
  name: string;
  email: string;
  role: string;
  isStaff: boolean;
  permissions: string[];
  customerId: string | null;
  csrfToken: string;
};

export type Media = {
  id: string;
  url: string;
  uploadId: string | null;
  alt: string;
  width: number | null;
  height: number | null;
  sample?: boolean;
};

export type Variant = {
  id: string;
  sku: string;
  sizeLabel: string;
  colorName: string;
  colorHex: string | null;
  priceMinor: number;
  stockQty?: number;
  madeToOrder: boolean;
  available: boolean;
  lowStock: boolean;
  active: boolean;
  sortOrder: number;
};

export type Availability = "in_stock" | "low_stock" | "made_to_order" | "out_of_stock";

export type Product = {
  id: string;
  slug: string;
  name: string;
  summary: string;
  description?: string;
  category: { id: string; slug: string; name: string } | null;
  garmentTypeKey: string | null;
  fabric: {
    key: string;
    name: string;
    composition: string;
    weightGsm: number | null;
    careInstructions: string;
    swatchUrl: string | null;
  } | null;
  fitNotes?: string;
  care?: string;
  measurementInfo?: string;
  requiresFitting: boolean;
  customizable: boolean;
  visibility: string;
  priceMinor: number;
  priceMaxMinor: number;
  featured: boolean;
  availability: Availability;
  media: Media[];
  variants: Variant[];
  sizes: string[];
  colors: string[];
  version: number;
  createdAt: string;
};

export type ListResponse<T> = { items: T[]; total: number; limit: number; offset: number };

export type Facets = {
  categories: { slug: string; name: string }[];
  sizes: string[];
  colors: string[];
  fabrics: { key: string; name: string }[];
  price: { min: number | null; max: number | null };
};

export type FabricColor = {
  id: string;
  key: string;
  name: string;
  hex: string;
  swatchUrl: string | null;
  stockStatus: string;
};

export type Fabric = {
  id: string;
  key: string;
  name: string;
  materialType: string;
  composition: string;
  weightGsm: number | null;
  textureDescription: string;
  season: string;
  careInstructions: string;
  priceImpactMinor: number;
  stockStatus: "available" | "low_stock" | "out_of_stock" | "discontinued" | "custom_order";
  stockMeters?: number | null;
  lowStockMeters?: number | null;
  swatchUrl: string | null;
  pbr: Pbr;
  suitableGarments: string[];
  sortOrder: number;
  active: boolean;
  colors: FabricColor[];
};

export type Pbr = {
  colorMap?: string;
  normalMap?: string;
  roughnessMap?: string;
  aoMap?: string;
  repeat?: number;
  roughness?: number;
  sheen?: number;
  clearcoat?: number;
  tint?: boolean;
};

export type GarmentType = {
  id: string;
  key: string;
  name: string;
  category: string;
  description: string;
  basePriceMinor: number;
  studioEnabled: boolean;
  bodyModelHint: "any" | "masculine" | "feminine";
  quoteOnly: boolean;
  sortOrder: number;
  active: boolean;
};

export type AssetParts = { show?: string[]; hide?: string[]; morphs?: Record<string, number> };

export type OptionValue = {
  id: string;
  key: string;
  name: string;
  description: string;
  priceMinor: number;
  assetParts: AssetParts;
  adjustments: Record<string, number>;
  isDefault: boolean;
  sortOrder: number;
  active: boolean;
};

export type OptionGroup = {
  id: string;
  key: string;
  name: string;
  section: string;
  selection: "single" | "number";
  required: boolean;
  minValue: number | null;
  maxValue: number | null;
  stepValue: number | null;
  defaultNumber: number | null;
  unit: string | null;
  sortOrder: number;
  active: boolean;
  values: OptionValue[];
};

export type MeasurementField = {
  key: string;
  label: string;
  bodyLocation: string;
  instruction: string;
  helperNote: string;
  diagramKey: string | null;
  kind: string;
  minMm: number;
  maxMm: number;
  required: boolean;
  sortOrder: number;
};

export type AssetFile = { lod: string; url: string; bytes: number; sha256: string };

export type Asset = {
  id: string;
  assetKey: string;
  version: number;
  kind: string;
  garmentTypeKey: string | null;
  status: string;
  files: AssetFile[];
  bodyCompat: Record<string, unknown>;
  supportedOptions: { baseHidden?: string[]; renders?: Record<string, string>; bodyModel?: string } & Record<
    string,
    unknown
  >;
  textureSetVersion: string;
  license: { source?: string; license?: string; author?: string };
  productionQuality: boolean;
  notes: string;
};

export type StudioConfig = {
  garment: GarmentType;
  groups: OptionGroup[];
  fitRules: FitRule[];
  sizes: { id: string; label: string; dims: Record<string, number> }[];
  measurements: MeasurementField[];
  asset: Asset | null;
  bodyAssets: Asset[];
};

export type DesignSnapshot = {
  schema: number;
  garment: { key: string; name: string };
  asset: { key: string; version: number } | null;
  selections: {
    group: string;
    groupName: string;
    section: string;
    value?: string;
    valueName?: string;
    number?: number;
    unit?: string;
    priceMinor: number;
  }[];
  fabric: {
    key: string;
    name: string;
    colorKey: string;
    colorName: string;
    colorHex: string;
    priceImpactMinor: number;
    stockStatus: string;
    pbr: Pbr;
  } | null;
  baseline: { type: string; label?: string };
  fitPreference: string;
  bodyModel: string;
  measurements: {
    versionId: string | null;
    source: string;
    unit: string;
    heightMm: number | null;
    valuesMm: Record<string, number>;
  } | null;
  fit: FitResult;
  price: {
    currency: string;
    baseMinor: number;
    optionsMinor: number;
    fabricMinor: number;
    totalMinor: number;
    estimate: boolean;
  };
  notes: string;
  warnings: string[];
  builtAt: string;
};

export type SavedDesign = {
  id: string;
  name: string;
  garment: string;
  version: number;
  versionId: string | null;
  snapshot: DesignSnapshot;
  assetAvailable: boolean;
  createdAt: string;
  updatedAt: string;
};

export type PortfolioProject = {
  id: string;
  slug: string;
  title: string;
  category: string;
  description: string;
  materials: string;
  tags: string[];
  videoUrl: string | null;
  featured: boolean;
  media: Media[];
  createdAt: string;
  customerPermission?: string;
  status?: string;
  sortOrder: number;
};

export type Testimonial = {
  id: string;
  customerName: string;
  quote: string;
  context: string;
  publishedAt: string | null;
};

export type HistoryItem = {
  oldStatus: string | null;
  newStatus: string;
  actor: string;
  note: string | null;
  createdAt: string;
};

export type OrderItem = {
  id: string;
  kind: "product" | "bespoke" | "fee";
  productId: string | null;
  designVersionId: string | null;
  name: string;
  description: string;
  snapshot: Record<string, unknown>;
  quantity: number;
  unitPriceMinor: number;
  totalMinor: number;
};

export type PaymentSummary = {
  id: string;
  purpose: "deposit" | "balance" | "full";
  provider: string;
  amountMinor: number;
  status: PaymentStatus;
  simulated: boolean;
  failureMessage: string | null;
  refundedMinor: number;
  createdAt: string;
  succeededAt: string | null;
};

export type PaymentStatus =
  | "created"
  | "pending"
  | "customer_action_required"
  | "processing"
  | "succeeded"
  | "failed"
  | "expired"
  | "cancelled"
  | "refunded"
  | "partially_refunded";

export type Order = {
  id: string;
  number: string;
  kind: "ready_made" | "bespoke";
  status: string;
  statusLabel: string;
  customerId: string;
  quoteId: string | null;
  quoteRevisionId: string | null;
  requestId: string | null;
  measurementVersionId: string | null;
  currency: string;
  subtotalMinor: number;
  discountMinor: number;
  deliveryMinor: number;
  taxMinor: number;
  totalMinor: number;
  depositRequiredMinor: number;
  amountPaidMinor: number;
  amountRefundedMinor: number;
  balanceMinor: number;
  paymentStatus: string;
  fulfillmentMethod: "pickup" | "local_delivery" | "courier";
  deliveryAddress: Record<string, string> | null;
  deliveryNote: string;
  deliveryStatus: string;
  contact: { name: string; phone: string; email?: string | null; preferredContact: string };
  customerNotes: string;
  urgency: string;
  dueDate: string | null;
  version: number;
  items: OrderItem[];
  history: HistoryItem[];
  notes: { id: string; visibility: string; body: string; author: string | null; createdAt: string }[];
  payments: PaymentSummary[];
  fittings: {
    id: string;
    appointmentId: string | null;
    notes?: string;
    customerNotes: string;
    adjustments: { area: string; change: string }[];
    createdAt: string;
  }[];
  appointments: { id: string; number: string; type: string; status: string; startsAt: string; endsAt: string }[];
  createdAt: string;
  updatedAt: string;
};

export type Payment = {
  id: string;
  orderId: string;
  purpose: string;
  provider: string;
  providerName: string;
  amountMinor: number;
  currency: string;
  status: PaymentStatus;
  simulated: boolean;
  failureMessage: string | null;
  payer: string;
  expiresAt: string;
  succeededAt: string | null;
  createdAt: string;
  existing?: boolean;
};

export type QuoteLine = { kind: string; description: string; quantity: number; unitMinor: number; totalMinor: number };

export type QuoteRevision = {
  id: string;
  revisionNo: number;
  currency: string;
  lines: QuoteLine[];
  subtotalMinor: number;
  discountMinor: number;
  deliveryMinor: number;
  taxMinor: number;
  taxRateBp: number;
  totalMinor: number;
  depositMinor: number;
  balanceMinor: number;
  expiresAt: string;
  customerNotes: string;
  terms: string;
  estimatedReadyDate: string | null;
  measurementVersionId: string | null;
  designVersionId: string | null;
  sentAt: string | null;
  createdAt: string;
};

export type Quote = {
  id: string;
  number: string;
  requestId: string | null;
  requestNumber: string | null;
  customerId: string;
  customerName: string;
  status: "draft" | "sent" | "accepted" | "declined" | "changes_requested" | "expired" | "withdrawn";
  current: QuoteRevision | null;
  acceptedRevisionId: string | null;
  decisionNote: string | null;
  decidedAt: string | null;
  orderId: string | null;
  revisions?: QuoteRevision[];
  history: HistoryItem[];
  expired: boolean;
  version: number;
  createdAt: string;
  updatedAt: string;
};

export type MeasurementVersion = {
  id: string;
  profileId: string | null;
  versionNo: number;
  source: "customer_entered" | "tailor_verified" | "imported" | "estimated";
  heightMm: number | null;
  bodyModel: string;
  fitPreference: string;
  valuesMm: Record<string, number>;
  entered: Record<string, number>;
  unit: "cm" | "in";
  notes: string;
  reviewFlags: { keys: string[]; message: string }[];
  verifiedBy: string | null;
  verifiedAt: string | null;
  createdAt: string;
};

export type MeasurementProfile = {
  id: string;
  name: string;
  isDefault: boolean;
  bodyModel: "masculine" | "feminine";
  unit: "cm" | "in";
  ageRange: string | null;
  fitPreference: "slim" | "regular" | "relaxed";
  current: MeasurementVersion | null;
  updatedAt: string;
};

export type RequestView = {
  id: string;
  number: string;
  customerId: string;
  status: string;
  garment: string;
  garmentName: string;
  occasion: string;
  occasionNote: string;
  measurementMode: string;
  measurementVersionId: string | null;
  verifiedMeasurementVersionId: string | null;
  measurements: MeasurementVersion | null;
  verifiedMeasurements: MeasurementVersion | null;
  bodyModel: string;
  fitPreference: string;
  fabricMode: string;
  fabricKey: string | null;
  fabricName: string | null;
  colorKey: string | null;
  designVersionId: string | null;
  design: DesignSnapshot | null;
  notes: string;
  desiredDate: string | null;
  dateFlexibility: string;
  urgency: string;
  contact: { name: string; phone: string; email: string | null; preferredContact: string };
  infoRequested: string | null;
  internalNotes?: string;
  supportThreadId: string | null;
  references: {
    uploadId: string;
    tag: string;
    note: string;
    url: string;
    thumb: string;
    removed: boolean;
    width: number | null;
    height: number | null;
  }[];
  quotes: {
    id: string;
    number: string;
    status: string;
    totalMinor: number | null;
    currency: string | null;
    expiresAt: string | null;
    updatedAt: string;
  }[];
  history: HistoryItem[];
  orderId: string | null;
  version: number;
  createdAt: string;
  updatedAt: string;
};

export type Appointment = {
  id: string;
  number: string;
  customerId: string;
  customerName: string;
  type: string;
  typeLabel: string;
  status: "booked" | "cancelled" | "completed" | "no_show";
  startsAt: string;
  endsAt: string;
  staffId: string | null;
  staffName: string | null;
  location: string;
  orderId: string | null;
  orderNumber: string | null;
  requestId: string | null;
  customerNotes: string;
  internalNotes?: string;
  contact: { name: string; phone: string; email: string | null; preferredContact: string };
  canChange: boolean;
  version: number;
};

export type SupportMessage = {
  id: string;
  authorType: "customer" | "staff" | "system";
  author: string | null;
  body: string;
  internal: boolean;
  attachments: { id: string; url: string; thumb: string }[];
  createdAt: string;
};

export type SupportThread = {
  id: string;
  number: string;
  customerId: string;
  customerName: string;
  subject: string;
  category: string;
  status: "open" | "pending" | "resolved";
  priority: "low" | "normal" | "high" | "urgent";
  assignedTo: string | null;
  assigneeName: string | null;
  orderId: string | null;
  orderNumber: string | null;
  requestId: string | null;
  requestNumber: string | null;
  unread: number;
  lastMessageAt: string;
  createdAt: string;
  messages?: SupportMessage[];
};

export type ContentBlocks = Record<string, unknown>;

export type SavedAddress = {
  id: string;
  label: string;
  recipient: string;
  phone: string;
  line1: string;
  line2: string;
  city: string;
  region: string;
  country: string;
  notes: string;
  isDefault: boolean;
};
