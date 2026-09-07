import SwiftUI

/// Ratio-enforced remote image components. The web mirrors live in
/// `internal/views/components/image.templ`
/// (`BannerImage` / `LandscapeImage` / `PortraitImage` / `SquareImage`).
///
/// Expected upload ratios — images should be uploaded already in
/// the ratio of the component that displays them, so nothing
/// important gets cropped away:
///
/// - BannerImage: 3:1 (e.g. 3000x1000) — exercise hero banners,
///   shared with the web's `components.BannerImage` so one upload
///   dimension works on both platforms.
/// - LandscapeImage: 16:9 (e.g. 1920x1080) — weight progress
///   photos and exercise images.
/// - PortraitImage: 3:4 (e.g. 1080x1440) — the photo comparison
///   sheet.
/// - SquareImage: 1:1 (e.g. 1080x1080).
///
/// A source with a different ratio still renders, but it is
/// centre-cropped to fill the frame (`.scaledToFill()` + clip) —
/// it is never stretched or distorted.
///
/// All four delegate to the private `RatioImage` implementation;
/// the ratio is the only difference between them. Each accepts an
/// optional `maxHeight` cap — the iOS mirror of the web's
/// `Class: "max-h-*"` caps — so a full-width frame can be kept
/// banner-height instead of growing with the container width.
/// Upload ratios are unchanged by the cap: the image just
/// centre-crops top/bottom.

/// Remote image locked to a 3:1 banner frame. Expects 3:1
/// uploads (e.g. 3000x1000). The exercise-hero ratio, shared
/// with the web's `components.BannerImage` so one upload
/// dimension works on both platforms — callers cap its width
/// (see `ExerciseHistoryView.heroMaxWidth`) and the frame stays
/// exactly 3:1 at every size.
struct BannerImage: View {
    let url: URL?
    var accessibilityLabel: String = ""
    var placeholderIcon: String = "photo"
    var cornerRadius: CGFloat = DSSpacing.cornerRadius
    var maxHeight: CGFloat? = nil

    var body: some View {
        RatioImage(
            ratio: 3.0 / 1.0,
            url: url,
            accessibilityLabel: accessibilityLabel,
            placeholderIcon: placeholderIcon,
            cornerRadius: cornerRadius,
            maxHeight: maxHeight
        )
    }
}

/// Remote image locked to a 16:9 landscape frame. Expects 16:9
/// uploads (e.g. 1920x1080).
struct LandscapeImage: View {
    let url: URL?
    var accessibilityLabel: String = ""
    var placeholderIcon: String = "photo"
    var cornerRadius: CGFloat = DSSpacing.cornerRadius
    var maxHeight: CGFloat? = nil

    var body: some View {
        RatioImage(
            ratio: 16.0 / 9.0,
            url: url,
            accessibilityLabel: accessibilityLabel,
            placeholderIcon: placeholderIcon,
            cornerRadius: cornerRadius,
            maxHeight: maxHeight
        )
    }
}

/// Remote image locked to a 3:4 portrait frame. Expects 3:4
/// uploads (e.g. 1080x1440).
struct PortraitImage: View {
    let url: URL?
    var accessibilityLabel: String = ""
    var placeholderIcon: String = "photo"
    var cornerRadius: CGFloat = DSSpacing.cornerRadius
    var maxHeight: CGFloat? = nil
    var showsLoadingIndicator: Bool = false

    var body: some View {
        RatioImage(
            ratio: 3.0 / 4.0,
            url: url,
            accessibilityLabel: accessibilityLabel,
            placeholderIcon: placeholderIcon,
            cornerRadius: cornerRadius,
            maxHeight: maxHeight,
            showsLoadingIndicator: showsLoadingIndicator
        )
    }
}

/// Remote image locked to a 1:1 square frame. Expects square
/// uploads (e.g. 1080x1080).
struct SquareImage: View {
    let url: URL?
    var accessibilityLabel: String = ""
    var placeholderIcon: String = "photo"
    var cornerRadius: CGFloat = DSSpacing.cornerRadius
    var maxHeight: CGFloat? = nil

    var body: some View {
        RatioImage(
            ratio: 1.0,
            url: url,
            accessibilityLabel: accessibilityLabel,
            placeholderIcon: placeholderIcon,
            cornerRadius: cornerRadius,
            maxHeight: maxHeight
        )
    }
}

/// Shared implementation behind the three ratio-enforced image
/// components. Loads the remote image with `AsyncImage`, fills the
/// aspect-locked frame (centre-cropping overflow, never
/// distorting), and falls back to a muted symbol tile while
/// loading / on failure.
private struct RatioImage: View {
    /// Width : height of the frame, e.g. 16.0 / 9.0.
    let ratio: CGFloat
    let url: URL?
    /// Accessibility label applied when non-empty; an empty label
    /// leaves the image out of the accessibility tree.
    let accessibilityLabel: String
    /// SF Symbol shown in the placeholder tile (loading / failure
    /// / no URL).
    let placeholderIcon: String
    let cornerRadius: CGFloat
    /// Optional height cap applied between the aspect lock and the
    /// clip, so the cover-crop cuts at the capped height (the
    /// window becomes wider than the ratio, exactly like the web
    /// frame's `max-h-*` cap). `nil` leaves the true ratio frame.
    let maxHeight: CGFloat?
    /// When true the loading state overlays a spinner on the
    /// placeholder; contexts with many small images leave it off
    /// so lists don't fill up with spinners.
    var showsLoadingIndicator: Bool = false

    var body: some View {
        if accessibilityLabel.isEmpty {
            framed
        } else {
            framed.accessibilityLabel(accessibilityLabel)
        }
    }

    private var framed: some View {
        // The size is defined by an inert spacer locked to the
        // ratio; the image is overlaid on it and clipped. Overlay
        // can never grow for its content, so the reported height
        // stays exactly width / ratio in every load state. Without
        // this, a scaledToFill image reports its natural-ratio size
        // upward and inflates the containing List row (~2x for a
        // 16:9 source in a 3:1 frame).
        Color.clear
            .aspectRatio(ratio, contentMode: .fit)
            .overlay { content }
            .frame(maxHeight: maxHeight)
            .clipped()
            .clipShape(RoundedRectangle(cornerRadius: cornerRadius, style: .continuous))
    }

    @ViewBuilder
    private var content: some View {
        if let url {
            AsyncImage(url: url) { phase in
                switch phase {
                case .success(let image):
                    image
                        .resizable()
                        .scaledToFill()
                case .empty:
                    if showsLoadingIndicator {
                        ZStack {
                            placeholder
                            ProgressView()
                        }
                    } else {
                        placeholder
                    }
                case .failure:
                    placeholder
                @unknown default:
                    placeholder
                }
            }
        } else {
            placeholder
        }
    }

    /// Muted tile with the placeholder symbol, matching the
    /// placeholder patterns used across the app's image slots.
    private var placeholder: some View {
        Rectangle()
            .fill(DSColors.surfaceElevated)
            .overlay {
                Image(systemName: placeholderIcon)
                    .font(.largeTitle)
                    .foregroundStyle(DSColors.textSecondary)
            }
    }
}
