import SwiftUI

/// Portrait comparison sheet for two weight photos in a single angle
/// slot. The older photo is revealed over the newer photo with a
/// draggable divider, matching the web comparison interaction
/// without a third-party image viewer or image-loading dependency.
///
/// The compared angle is switchable via the segmented control when
/// both entries share more than one angle; angles either entry
/// lacks are disabled. Switching angles is local (both entries
/// already carry every angle's URL) so no refetch is needed.
struct WeightComparisonView: View {
    let comparison: WeightCompareResponse
    let weightUnit: String

    @Environment(\.dismiss) private var dismiss
    @State private var revealPosition: CGFloat = 0.5
    @State private var selectedAngle: WeightEntryDTO.PhotoAngle

    init(comparison: WeightCompareResponse, weightUnit: String) {
        self.comparison = comparison
        self.weightUnit = weightUnit
        // Default to the server-compared angle; fall back to the
        // first shared angle when the server string is unknown
        // (e.g. an older build that omits it).
        let initial = WeightEntryDTO.PhotoAngle(rawValue: comparison.angle) ?? .front
        let shared = WeightComparisonView.sharedAngles(before: comparison.before, after: comparison.after)
        _selectedAngle = State(initialValue: shared.contains(initial) ? initial : (shared.first ?? .front))
    }

    /// Angles both entries have photos for, in preference order.
    private var sharedAngles: [WeightEntryDTO.PhotoAngle] {
        Self.sharedAngles(before: comparison.before, after: comparison.after)
    }

    private static func sharedAngles(before: WeightEntryDTO, after: WeightEntryDTO) -> [WeightEntryDTO.PhotoAngle] {
        WeightEntryDTO.PhotoAngle.allCases.filter { before.hasPhoto(for: $0) && after.hasPhoto(for: $0) }
    }

    private var beforeLabel: String {
        comparison.before.createdAt.formatted(.dateTime.day().month(.abbreviated).year())
    }

    private var afterLabel: String {
        comparison.after.createdAt.formatted(.dateTime.day().month(.abbreviated).year())
    }

    var body: some View {
        NavigationStack {
            VStack(spacing: DSSpacing.md) {
                if sharedAngles.count > 1 {
                    Picker("Angle", selection: $selectedAngle) {
                        ForEach(WeightEntryDTO.PhotoAngle.allCases, id: \.self) { angle in
                            Text(angle.rawValue.capitalized)
                                .tag(angle)
                                .disabled(!sharedAngles.contains(angle))
                        }
                    }
                    .pickerStyle(.segmented)
                    .accessibilityLabel("Photo angle")
                }

                comparisonImage
                    .aspectRatio(3.0 / 4.0, contentMode: .fit)
                    .clipShape(RoundedRectangle(cornerRadius: DSSpacing.cornerRadius, style: .continuous))
                    .overlay(
                        RoundedRectangle(cornerRadius: DSSpacing.cornerRadius, style: .continuous)
                            .stroke(DSColors.separator, lineWidth: 0.5)
                    )
                    .id(selectedAngle)

                HStack {
                    photoLabel(title: "Before", date: beforeLabel, entry: comparison.before)
                    Spacer()
                    photoLabel(title: "After", date: afterLabel, entry: comparison.after)
                }

                Text("Drag the divider to compare")
                    .font(.footnote)
                    .foregroundStyle(DSColors.textSecondary)
            }
            .padding(DSSpacing.md)
            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
            .background(DSColors.background.ignoresSafeArea())
            .navigationTitle("Compare \(selectedAngle.rawValue.capitalized)")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) {
                    Button("Done") { dismiss() }
                }
            }
        }
        .presentationDragIndicator(.visible)
    }

    private var comparisonImage: some View {
        GeometryReader { geometry in
            let width = geometry.size.width
            let height = geometry.size.height

            ZStack(alignment: .leading) {
                // Both photos render through PortraitImage — the
                // ratio-enforced component formalising the 3:4
                // frame this sheet has always used. cornerRadius 0
                // because the ZStack below clips and strokes the
                // whole comparison as one rounded shape.
                PortraitImage(
                    url: URL(string: comparison.after.photoURL(for: selectedAngle)),
                    cornerRadius: 0,
                    showsLoadingIndicator: true
                )
                .frame(width: width, height: height)

                PortraitImage(
                    url: URL(string: comparison.before.photoURL(for: selectedAngle)),
                    cornerRadius: 0,
                    showsLoadingIndicator: true
                )
                .frame(width: width, height: height)
                .mask(alignment: .leading) {
                    Rectangle()
                        .frame(width: width * revealPosition, height: height)
                }

                Rectangle()
                    .fill(BrandColors.brandOrange)
                    .frame(width: 3, height: height)
                    .shadow(color: .black.opacity(0.35), radius: 4)
                    .offset(x: width * revealPosition - 1.5)

                HStack {
                    sliderHandle
                        .offset(x: width * revealPosition - 22)
                    Spacer()
                }
            }
            .contentShape(Rectangle())
            .gesture(
                DragGesture(minimumDistance: 0)
                    .onChanged { value in
                        revealPosition = min(max(value.location.x / max(width, 1), 0), 1)
                    }
            )
            .accessibilityElement(children: .ignore)
            .accessibilityLabel("Photo comparison slider")
            .accessibilityValue("\(Int(revealPosition * 100)) percent before photo")
            .accessibilityAdjustableAction { direction in
                let step: CGFloat = 0.05
                switch direction {
                case .increment:
                    revealPosition = min(revealPosition + step, 1)
                case .decrement:
                    revealPosition = max(revealPosition - step, 0)
                @unknown default:
                    break
                }
            }
        }
    }

    private var sliderHandle: some View {
        Image(systemName: "chevron.left.2")
            .font(.caption.weight(.bold))
            .foregroundStyle(.white)
            .frame(width: 44, height: 44)
            .background(BrandColors.brandOrange, in: Circle())
            .shadow(color: .black.opacity(0.3), radius: 4)
    }

    private func photoLabel(title: String, date: String, entry: WeightEntryDTO) -> some View {
        VStack(alignment: title == "Before" ? .leading : .trailing, spacing: DSSpacing.xxs) {
            Text(title)
                .font(.subheadline.weight(.semibold))
                .foregroundStyle(DSColors.text)
            Text(date)
                .font(.caption)
                .foregroundStyle(DSColors.textSecondary)
            Text(entry.formattedWeight(in: weightUnit))
                .font(.caption.monospacedDigit())
                .foregroundStyle(DSColors.textSecondary)
        }
    }
}
