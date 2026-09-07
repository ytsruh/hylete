import SwiftUI
import UIKit

/// Full-screen celebration for a newly completed goal. The particle
/// effect is backed by Core Animation so it stays lightweight and does
/// not require a third-party package.
struct GoalCompletionCelebration: View {
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    @Environment(\.colorScheme) private var colorScheme

    let trigger: Int

    var body: some View {
        if reduceMotion {
            ReducedMotionSuccessView(trigger: trigger)
        } else {
            ConfettiView(trigger: trigger, isDark: colorScheme == .dark)
        }
    }
}

/// UIKit bridge for the Core Animation confetti emitter. The trigger is
/// an incrementing value rather than a Boolean so repeated completions
/// always produce a new burst.
private struct ConfettiView: UIViewRepresentable {
    let trigger: Int
    let isDark: Bool

    func makeCoordinator() -> Coordinator {
        Coordinator()
    }

    func makeUIView(context: Context) -> ConfettiContainerView {
        let view = ConfettiContainerView(isDark: isDark)
        context.coordinator.view = view
        return view
    }

    func updateUIView(_ view: ConfettiContainerView, context: Context) {
        view.isDark = isDark
        guard context.coordinator.lastTrigger != trigger else { return }
        context.coordinator.lastTrigger = trigger
        view.fire()
    }

    final class Coordinator {
        weak var view: ConfettiContainerView?
        var lastTrigger = 0
    }
}

private final class ConfettiContainerView: UIView {
    var isDark: Bool

    init(isDark: Bool) {
        self.isDark = isDark
        super.init(frame: .zero)
        isUserInteractionEnabled = false
    }

    @available(*, unavailable)
    required init?(coder: NSCoder) {
        fatalError("init(coder:) has not been implemented")
    }

    private var colors: [UIColor] {
        [
            UIColor(red: 0.961, green: 0.286, blue: 0.000, alpha: 1),
            UIColor(red: 0.72, green: 0.74, blue: 0.78, alpha: 1),
            isDark ? .white : .black
        ]
    }

    override class var layerClass: AnyClass { CALayer.self }

    func fire() {
        layoutIfNeeded()

        let center = makeEmitter(
            position: CGPoint(x: bounds.midX, y: -8),
            particleRate: 45,
            velocity: 160,
            spread: 0.35
        )
        let left = makeEmitter(
            position: CGPoint(x: bounds.width * 0.25, y: -8),
            particleRate: 18,
            velocity: 140,
            spread: 0.5
        )
        let right = makeEmitter(
            position: CGPoint(x: bounds.width * 0.75, y: -8),
            particleRate: 18,
            velocity: 140,
            spread: 0.5
        )

        layer.addSublayer(center)
        stopEmitting(center)

        // A short delay gives the lighter top-down effect a little rhythm
        // without recreating the dense, mid-screen explosion.
        DispatchQueue.main.asyncAfter(deadline: .now() + 0.2) { [weak self] in
            guard let self else { return }
            self.layer.addSublayer(left)
            self.layer.addSublayer(right)
            self.stopEmitting(left)
            self.stopEmitting(right)
        }

        DispatchQueue.main.asyncAfter(deadline: .now() + 3.5) {
            [center, left, right].forEach { $0.removeFromSuperlayer() }
        }
    }

    private func stopEmitting(_ emitter: CAEmitterLayer) {
        DispatchQueue.main.asyncAfter(deadline: .now() + 0.08) {
            emitter.birthRate = 0
        }
    }

    private func makeEmitter(
        position: CGPoint,
        particleRate: Float,
        velocity: CGFloat,
        spread: CGFloat
    ) -> CAEmitterLayer {
        let emitter = CAEmitterLayer()
        emitter.emitterPosition = position
        emitter.emitterShape = .point
        emitter.emitterMode = .outline
        emitter.renderMode = .oldestFirst
        emitter.birthRate = 1
        emitter.emitterCells = colors.enumerated().map { index, color in
            let cell = CAEmitterCell()
            cell.birthRate = particleRate / Float(colors.count)
            cell.lifetime = 3.0
            cell.lifetimeRange = 0.8
            cell.velocity = velocity
            cell.velocityRange = abs(velocity) * 0.25
            cell.emissionLongitude = .pi / 2
            cell.emissionRange = spread
            cell.spin = 2
            cell.spinRange = 5
            cell.scale = 0.6
            cell.scaleRange = 0.25
            cell.yAcceleration = 100
            cell.xAcceleration = index.isMultiple(of: 2) ? -20 : 20
            cell.contents = Self.particleImage(color: color).cgImage
            return cell
        }
        return emitter
    }

    private static func particleImage(color: UIColor) -> UIImage {
        let renderer = UIGraphicsImageRenderer(size: CGSize(width: 7, height: 12))
        return renderer.image { context in
            color.setFill()
            context.cgContext.fill(CGRect(x: 0, y: 0, width: 7, height: 12))
        }
    }
}

/// A restrained alternative for users who have Reduce Motion enabled.
private struct ReducedMotionSuccessView: View {
    let trigger: Int

    @State private var visible = false

    var body: some View {
        Image(systemName: "checkmark.circle.fill")
            .font(.system(size: 72, weight: .semibold))
            .foregroundStyle(DSColors.success)
            .shadow(color: .black.opacity(0.18), radius: 10, y: 4)
            .opacity(visible ? 1 : 0)
            .scaleEffect(visible ? 1 : 0.85)
            .onChange(of: trigger, initial: true) {
                guard trigger > 0 else { return }
                visible = true
                DispatchQueue.main.asyncAfter(deadline: .now() + 1.2) {
                    visible = false
                }
            }
    }
}
