import SwiftUI
import UIKit

/// Full-screen, pinch-to-zoom image viewer. Presented with
/// `.fullScreenCover(item:)` — see `ExerciseHistoryView`, which
/// opens it when the hero banner is tapped. The web mirror of
/// this behaviour is the click-to-dialog on
/// `internal/views/exercise/history.templ`.
///
/// Zooming is delegated to a `UIScrollView`
/// (`ZoomableScrollView`) so all the native behaviours come for
/// free: pinch-zoom anchored under the fingers (1x-5x), panning
/// while zoomed, rubber-band bounce, and momentum. A double-tap
/// toggles between 1x and 2.5x. The image is downloaded with
/// `URLSession` (URLCache-backed, so re-opening is instant) and
/// decoded on the main actor with a spinner while in flight.
///
/// All frameworks used are system frameworks — no third-party
/// image library, per the project's dependency rule.
struct ImageViewer: View {
    /// Image to display. Nil is a programming-error guard (the
    /// viewer is only presented with a URL) — shows the muted
    /// placeholder tile rather than crashing.
    let url: URL?

    @Environment(\.dismiss) private var dismiss

    var body: some View {
        ZStack(alignment: .topTrailing) {
            Color.black.ignoresSafeArea()

            if let url {
                ZoomableScrollView(url: url)
            } else {
                Image(systemName: "photo")
                    .font(.largeTitle)
                    .foregroundStyle(.white.opacity(0.5))
            }

            Button(action: { dismiss() }) {
                Image(systemName: "xmark")
                    .font(.system(size: 14, weight: .bold))
                    .foregroundStyle(.white)
                    .padding(10)
                    .background(.white.opacity(0.2), in: Circle())
            }
            .padding(.top, DSSpacing.sm)
            .padding(.trailing, DSSpacing.md)
            .accessibilityLabel("Close")
        }
        .preferredColorScheme(.dark)
    }
}

/// UIKit bridge giving the viewer a native `UIScrollView` zoom
/// surface. The image view is the zooming view, sized to the
/// scroll view's bounds with `.scaleAspectFit`, so a wide image
/// letterboxes at 1x and pinch-zoom scales it up from there.
private struct ZoomableScrollView: UIViewRepresentable {
    let url: URL

    func makeUIView(context: Context) -> UIScrollView {
        let scrollView = UIScrollView()
        scrollView.delegate = context.coordinator
        scrollView.minimumZoomScale = 1.0
        scrollView.maximumZoomScale = 5.0
        scrollView.bouncesZoom = true
        scrollView.showsVerticalScrollIndicator = false
        scrollView.showsHorizontalScrollIndicator = false
        scrollView.backgroundColor = .clear

        let imageView = UIImageView(image: nil)
        imageView.contentMode = .scaleAspectFit
        imageView.frame = scrollView.bounds
        // Track the scroll view's bounds until the SwiftUI host
        // sizes the representable, and across size changes.
        imageView.autoresizingMask = [.flexibleWidth, .flexibleHeight]
        imageView.isAccessibilityElement = true
        imageView.accessibilityLabel = "Exercise image"
        scrollView.addSubview(imageView)

        let spinner = UIActivityIndicatorView(style: .large)
        spinner.color = .systemGray
        spinner.frame = CGRect(x: 0, y: 0, width: 44, height: 44)
        spinner.center = scrollView.center
        spinner.autoresizingMask = [.flexibleLeftMargin, .flexibleRightMargin, .flexibleTopMargin, .flexibleBottomMargin]
        scrollView.addSubview(spinner)
        spinner.startAnimating()

        // Double-tap toggles between fit-scale and 2.5x, zoomed
        // around the tapped point.
        let doubleTap = UITapGestureRecognizer(
            target: context.coordinator,
            action: #selector(Coordinator.handleDoubleTap(_:))
        )
        doubleTap.numberOfTapsRequired = 2
        doubleTap.delegate = context.coordinator
        scrollView.addGestureRecognizer(doubleTap)

        context.coordinator.attach(scrollView: scrollView, imageView: imageView, spinner: spinner)
        context.coordinator.load(url: url)
        return scrollView
    }

    func updateUIView(_ uiView: UIScrollView, context: Context) {
        context.coordinator.reloadIfNeeded(url: url)
    }

    static func dismantleUIView(_ uiView: UIScrollView, coordinator: Coordinator) {
        coordinator.cancelLoad()
    }

    func makeCoordinator() -> Coordinator {
        Coordinator()
    }

    /// Owns the download task, the double-tap behaviour, and the
    /// delegate callbacks for the scroll view.
    final class Coordinator: NSObject, UIScrollViewDelegate {
        weak var scrollView: UIScrollView?
        weak var imageView: UIImageView?
        weak var spinner: UIActivityIndicatorView?
        var dataTask: URLSessionDataTask?
        var loadedURL: URL?

        func attach(scrollView: UIScrollView, imageView: UIImageView, spinner: UIActivityIndicatorView) {
            self.scrollView = scrollView
            self.imageView = imageView
            self.spinner = spinner
        }

        func load(url: URL) {
            guard loadedURL != url else { return }
            cancelLoad()
            loadedURL = url
            spinner?.startAnimating()
            imageView?.image = nil

            let task = URLSession.shared.dataTask(with: url) { [weak self] data, _, _ in
                DispatchQueue.main.async {
                    guard let self, self.loadedURL == url else { return }
                    defer { self.spinner?.stopAnimating() }
                    // Decode on the main queue: the source images
                    // are small enough (max 2400x800) that the
                    // main-thread decode is not perceptible, and
                    // it avoids SwiftUI/UIView threading traps.
                    if let data, let image = UIImage(data: data) {
                        UIView.transition(
                            with: self.imageView ?? UIView(),
                            duration: 0.2,
                            options: .transitionCrossDissolve
                        ) {
                            self.imageView?.image = image
                        }
                    }
                }
            }
            dataTask = task
            task.resume()
        }

        /// Re-downloads only when the presented URL changed.
        func reloadIfNeeded(url: URL) {
            guard loadedURL != url else { return }
            load(url: url)
        }

        func cancelLoad() {
            dataTask?.cancel()
            dataTask = nil
        }

        func viewForZooming(in scrollView: UIScrollView) -> UIView? {
            imageView
        }

        @objc func handleDoubleTap(_ gesture: UITapGestureRecognizer) {
            guard let scrollView else { return }
            if scrollView.zoomScale > scrollView.minimumZoomScale + 0.01 {
                scrollView.setZoomScale(scrollView.minimumZoomScale, animated: true)
                return
            }
            // A rect sized bounds/targetScale makes UIScrollView
            // land on exactly 2.5x, centred on the tapped point.
            // zoom(to:) clamps to the configured min/max scales.
            let targetScale: CGFloat = 2.5
            let size = CGSize(
                width: scrollView.bounds.width / targetScale,
                height: scrollView.bounds.height / targetScale
            )
            let point = gesture.location(in: imageView)
            let origin = CGPoint(x: point.x - size.width / 2, y: point.y - size.height / 2)
            scrollView.zoom(to: CGRect(origin: origin, size: size), animated: true)
        }
    }
}

extension ZoomableScrollView.Coordinator: UIGestureRecognizerDelegate {
    /// Let the double-tap recognise alongside the scroll view's
    /// built-in pinch/pan gesture recognisers.
    func gestureRecognizer(
        _ gestureRecognizer: UIGestureRecognizer,
        shouldRecognizeSimultaneouslyWith otherGestureRecognizer: UIGestureRecognizer
    ) -> Bool {
        true
    }
}
