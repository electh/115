export function InfiniteFeedStatus() {
  return (
    <div className="listing-infinite-status" role="status" aria-live="polite">
      <span
        className="video-grid-refresh-overlay__spinner"
        aria-hidden="true"
      />
      <span>正在加载更多</span>
    </div>
  );
}
