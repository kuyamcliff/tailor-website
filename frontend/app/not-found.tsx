import Link from "next/link";

export default function NotFound() {
  return (
    <section className="section">
      <div className="container-narrow stack-lg">
        <span className="eyebrow">Page not found</span>
        <h1 className="display-2">We could not find that page.</h1>
        <p className="lede">It may have moved, or the link may be incomplete.</p>
        <div className="row-wrap">
          <Link href="/" className="btn btn-primary">
            Back to home
          </Link>
          <Link href="/shop" className="btn">
            Visit the shop
          </Link>
        </div>
      </div>
    </section>
  );
}
